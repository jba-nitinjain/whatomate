package handlers

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nikyjain/whatomate/internal/models"
)

// rsvpAlreadyResponded reports whether a completed (non-pending) response already
// exists for the phone — either as the responder's number or as a recorded spouse
// mobile — so a duplicate submission can be turned away.
func (a *App) rsvpAlreadyResponded(event *models.RSVPEvent, phone string) bool {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return false
	}
	q := a.DB.Model(&models.RSVPResponse{}).
		Where("rsvp_event_id = ? AND attendance <> ?", event.ID, models.RSVPAttendancePending)
	if strings.TrimSpace(event.SpouseMobileField) != "" {
		q = q.Where("phone_number = ? OR answers->>? = ?", phone, event.SpouseMobileField, phone)
	} else {
		q = q.Where("phone_number = ?", phone)
	}
	var count int64
	q.Count(&count)
	return count > 0
}

// rsvpEventIDKey is the SessionData key that ties a chatbot session to an RSVP event.
const rsvpEventIDKey = "_rsvp_event_id"

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
func (a *App) seedPendingRSVPResponse(orgID uuid.UUID, event *models.RSVPEvent, contactID uuid.UUID, phone string) {
	var existing models.RSVPResponse
	if err := a.DB.Where("rsvp_event_id = ? AND contact_id = ?", event.ID, contactID).First(&existing).Error; err == nil {
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
	}).Error
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

	// Derive attendance from the configured field + map.
	attendance := models.RSVPAttendancePending
	if event.AttendanceField != "" {
		if val, ok := session.SessionData[event.AttendanceField]; ok {
			attendance = mapAttendance(event.AttendanceMap, val)
		}
	}

	now := time.Now()
	updates := map[string]interface{}{
		"answers":      answers,
		"attendance":   attendance,
		"responded_at": now,
	}
	// Upsert: update existing (pending) row, else create.
	res := a.DB.Model(&models.RSVPResponse{}).
		Where("rsvp_event_id = ? AND contact_id = ?", event.ID, session.ContactID).
		Updates(updates)
	if res.Error == nil && res.RowsAffected == 0 {
		_ = a.DB.Create(&models.RSVPResponse{
			BaseModel:      models.BaseModel{ID: uuid.New()},
			RSVPEventID:    event.ID,
			OrganizationID: session.OrganizationID,
			ContactID:      session.ContactID,
			PhoneNumber:    session.PhoneNumber,
			Attendance:     attendance,
			Answers:        answers,
			RespondedAt:    &now,
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
