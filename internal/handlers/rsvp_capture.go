package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nikyjain/whatomate/internal/models"
)

// normalizePhoneDigits strips everything except digits from a phone number.
func normalizePhoneDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// phoneMatchSuffix returns the last 10 digits of a number (or all of them when
// shorter), used to compare numbers regardless of country-code/formatting.
func phoneMatchSuffix(s string) string {
	d := normalizePhoneDigits(s)
	if len(d) > 10 {
		return d[len(d)-10:]
	}
	return d
}

// rsvpAlreadyResponded reports whether a completed (non-pending) response already
// exists for the phone — either as the responder's number or as a recorded spouse
// mobile. Matching is by trailing digits so differing formats (spaces, +, country
// code) still match, so any number that already submitted is turned away.
func (a *App) rsvpAlreadyResponded(event *models.RSVPEvent, phone string) bool {
	suffix := phoneMatchSuffix(phone)
	if suffix == "" {
		return false
	}
	like := "%" + suffix
	q := a.DB.Model(&models.RSVPResponse{}).
		Where("rsvp_event_id = ? AND (responded_at IS NOT NULL OR attendance <> ?)", event.ID, models.RSVPAttendancePending)
	if strings.TrimSpace(event.SpouseMobileField) != "" {
		q = q.Where("phone_number LIKE ? OR answers->>? LIKE ?", like, event.SpouseMobileField, like)
	} else {
		q = q.Where("phone_number LIKE ?", like)
	}
	var count int64
	q.Count(&count)
	return count > 0
}

// rsvpEventIDKey is the SessionData key that ties a chatbot session to an RSVP event.
const rsvpEventIDKey = "_rsvp_event_id"

// rsvpFollowUpKey marks a session as a follow-up: it tops up an existing response
// rather than making a new one. The "_" prefix keeps it out of stored answers via
// the existing filter in finalizeRSVPFromSession.
const rsvpFollowUpKey = "_rsvp_followup"

// mergeRSVPAnswers overlays incoming answers onto existing ones. Incoming wins per
// key so a guest can correct themselves; keys absent from incoming survive
// untouched. Returns a new map - neither input is mutated.
func mergeRSVPAnswers(existing, incoming models.JSONB) models.JSONB {
	merged := models.JSONB{}
	for k, v := range existing {
		merged[k] = v
	}
	for k, v := range incoming {
		merged[k] = v
	}
	return merged
}

// rsvpShouldBlockDuplicate decides whether to turn a sender away with the event's
// DuplicateMessage. A follow-up deliberately targets people who already responded,
// so the guard must not apply to it - but it still protects the main RSVP.
func rsvpShouldBlockDuplicate(isFollowUp, alreadyResponded bool) bool {
	return !isFollowUp && alreadyResponded
}

// rsvpEventForFlow returns the active RSVP event linked to a flow, or nil.
func (a *App) rsvpEventForFlow(orgID, flowID uuid.UUID) *models.RSVPEvent {
	var event models.RSVPEvent
	if err := a.DB.Where("organization_id = ? AND flow_id = ? AND status = ?",
		orgID, flowID, models.RSVPEventStatusActive).First(&event).Error; err != nil {
		return nil
	}
	return &event
}

// seedPendingRSVPResponse creates a pending response row for a contact entering an event.
// No-op if a row already exists (does not overwrite an answered row).
func (a *App) seedPendingRSVPResponse(orgID uuid.UUID, event *models.RSVPEvent, contactID uuid.UUID, phone string, sources ...models.RSVPGuestSource) {
	source := models.RSVPGuestSourceAPI
	if len(sources) > 0 && sources[0] != "" {
		source = sources[0]
	}
	var existing models.RSVPResponse
	// Unscoped so a previously deleted row is found and revived rather than
	// colliding with the unique (event, contact) index on create.
	if err := a.DB.Unscoped().Where("rsvp_event_id = ? AND contact_id = ?", event.ID, contactID).First(&existing).Error; err == nil {
		if existing.DeletedAt.Valid {
			a.DB.Unscoped().Model(&models.RSVPResponse{}).Where("id = ?", existing.ID).
				Updates(map[string]interface{}{
					"deleted_at":   nil,
					"attendance":   models.RSVPAttendancePending,
					"answers":      models.JSONB{},
					"phone_number": phone,
					"responded_at": nil,
					"source":       source,
				})
		}
		return
	}
	_ = a.DB.Create(&models.RSVPResponse{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		RSVPEventID:    event.ID,
		OrganizationID: orgID,
		ContactID:      contactID,
		PhoneNumber:    phone,
		Attendance:     models.RSVPAttendancePending,
		Answers:        models.JSONB{},
		Source:         source,
	}).Error
}

func (a *App) rsvpGuestListed(eventID, contactID uuid.UUID) bool {
	var count int64
	a.DB.Model(&models.RSVPResponse{}).
		Where("rsvp_event_id = ? AND contact_id = ?", eventID, contactID).Count(&count)
	return count > 0
}

func (a *App) markRSVPStarted(eventID, contactID uuid.UUID) {
	now := time.Now().UTC()
	a.DB.Model(&models.RSVPResponse{}).
		Where("rsvp_event_id = ? AND contact_id = ? AND rsvp_started_at IS NULL", eventID, contactID).
		Update("rsvp_started_at", now)
}

// finalizeRSVPFromSession maps completed-flow SessionData into the RSVPResponse.
// No-op if the session is not tied to an RSVP event.
func (a *App) finalizeRSVPFromSession(session *models.ChatbotSession) {
	if session == nil || session.SessionData == nil {
		return
	}
	raw, ok := session.SessionData[rsvpEventIDKey].(string)
	if !ok || raw == "" {
		return
	}
	eventID, err := uuid.Parse(raw)
	if err != nil {
		return
	}
	var event models.RSVPEvent
	if err := a.DB.Where("id = ? AND organization_id = ?", eventID, session.OrganizationID).
		First(&event).Error; err != nil {
		return
	}

	// Build answers map (exclude internal keys prefixed with '_').
	answers := models.JSONB{}
	for k, v := range session.SessionData {
		if len(k) > 0 && k[0] == '_' {
			continue
		}
		answers[k] = v
	}

	// Store the spouse mobile as digits so duplicate matching works regardless of
	// how the guest typed it. A bare 10-digit Indian number is prefixed with 91 so
	// it matches the WhatsApp phone-number format guests message from.
	if event.SpouseMobileField != "" {
		if s, ok := answers[event.SpouseMobileField].(string); ok {
			if d := normalizePhoneDigits(s); d != "" {
				if len(d) == 10 {
					d = "91" + d
				}
				answers[event.SpouseMobileField] = d
			}
		}
	}

	// Derive attendance from the configured field + map.
	attendance := models.RSVPAttendancePending
	if event.AttendanceField != "" {
		if val, ok := session.SessionData[event.AttendanceField]; ok {
			attendance = mapAttendance(event.AttendanceMap, val)
		}
	}

	now := time.Now()

	// A follow-up tops up an existing response: it must never recompute
	// attendance or move responded_at, since the guest answered days ago and
	// only told us one extra thing now. It also must never create a row -
	// a follow-up with no existing response is a bug, not a new guest.
	//
	// If the key is present but isn't a bool, that's a programming error (e.g.
	// the flag got written as a string or int somewhere) - refuse to act rather
	// than silently falling through to the destructive replace path below.
	var isFollowUp bool
	if v, present := session.SessionData[rsvpFollowUpKey]; present {
		isFollowUp, ok = v.(bool)
		if !ok {
			a.Log.Error("RSVP follow-up flag has unexpected type; refusing to finalize",
				"session_id", session.ID, "type", fmt.Sprintf("%T", v))
			return
		}
	}

	updates := map[string]interface{}{}
	if isFollowUp {
		var current models.RSVPResponse
		if err := a.DB.Where("rsvp_event_id = ? AND contact_id = ?", event.ID, session.ContactID).
			First(&current).Error; err != nil {
			a.Log.Warn("RSVP follow-up has no existing response; ignoring",
				"event_id", event.ID, "contact_id", session.ContactID)
			return
		}
		updates["answers"] = mergeRSVPAnswers(current.Answers, answers)
	} else {
		updates["answers"] = answers
		updates["attendance"] = attendance
		updates["responded_at"] = now
		updates["deleted_at"] = nil // revive a soft-deleted row rather than colliding on create
	}
	// Upsert: update existing (pending or soft-deleted) row, else create. Unscoped
	// so a previously deleted row for this contact is reused.
	res := a.DB.Unscoped().Model(&models.RSVPResponse{}).
		Where("rsvp_event_id = ? AND contact_id = ?", event.ID, session.ContactID).
		Updates(updates)
	if res.Error == nil && res.RowsAffected == 0 && !isFollowUp {
		_ = a.DB.Create(&models.RSVPResponse{
			BaseModel:      models.BaseModel{ID: uuid.New()},
			RSVPEventID:    event.ID,
			OrganizationID: session.OrganizationID,
			ContactID:      session.ContactID,
			PhoneNumber:    session.PhoneNumber,
			Attendance:     attendance,
			Answers:        answers,
			RespondedAt:    &now,
			Source:         models.RSVPGuestSourceOpenKeyword,
		}).Error
	}
}

func mapAttendance(m models.JSONB, raw interface{}) models.RSVPAttendance {
	s, _ := raw.(string)
	if m != nil {
		if mapped, ok := m[s].(string); ok {
			s = mapped
		}
	}
	switch models.RSVPAttendance(s) {
	case models.RSVPAttendanceYes:
		return models.RSVPAttendanceYes
	case models.RSVPAttendanceNo:
		return models.RSVPAttendanceNo
	case models.RSVPAttendanceMaybe:
		return models.RSVPAttendanceMaybe
	default:
		return models.RSVPAttendancePending
	}
}

// FinalizeRSVPFromSessionForTest exposes finalizeRSVPFromSession for tests.
func (a *App) FinalizeRSVPFromSessionForTest(s *models.ChatbotSession) { a.finalizeRSVPFromSession(s) }
