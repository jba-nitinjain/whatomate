package handlers

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

type repromptTarget struct {
	ContactID    uuid.UUID  `json:"-"`
	Phone        string     `json:"phone"`
	Name         string     `json:"name"`
	Reason       string     `json:"reason"`
	RepromptedAt *time.Time `json:"reprompted_at,omitempty"`
}

// computeRepromptTargets returns the guests whose RSVP is incomplete for the event
// and should be re-sent the flow: responses that never reached the spouse question
// or lack the spouse mobile, plus contacts who tapped a button that was never
// recorded (no response row). Fully-completed responses are excluded.
func (a *App) computeRepromptTargets(orgID uuid.UUID, event *models.RSVPEvent) ([]repromptTarget, error) {
	var responses []models.RSVPResponse
	if err := a.DB.Where("organization_id = ? AND rsvp_event_id = ?", orgID, event.ID).
		Find(&responses).Error; err != nil {
		return nil, err
	}

	seen := map[uuid.UUID]bool{}
	targets := []repromptTarget{}

	contactInfo := func(id uuid.UUID) (string, string) {
		var c models.Contact
		if err := a.DB.Where("id = ? AND organization_id = ?", id, orgID).First(&c).Error; err != nil {
			return "", ""
		}
		return c.PhoneNumber, c.ProfileName
	}

	// Incomplete responses: pending, or answered attendance but no spouse answer,
	// or spouse attending but no spouse mobile recorded.
	for i := range responses {
		r := &responses[i]
		answers := map[string]interface{}(r.Answers)
		_, hasSpouseAtt := answers["spouse_attendance"]
		spouseMobile, _ := answers["spouse_mobile"].(string)
		spouseAtt, _ := answers["spouse_attendance"].(string)

		incomplete := r.Attendance == models.RSVPAttendancePending ||
			!hasSpouseAtt ||
			(spouseAtt == "yes" && len(normalizePhoneDigits(spouseMobile)) < 10)
		if !incomplete || seen[r.ContactID] {
			continue
		}
		seen[r.ContactID] = true
		phone, name := contactInfo(r.ContactID)
		targets = append(targets, repromptTarget{ContactID: r.ContactID, Phone: phone, Name: name, Reason: "incomplete", RepromptedAt: r.RepromptedAt})
	}

	// Contacts who sent a button reply (template tap) but have no response row.
	var msgs []models.Message
	if err := a.DB.Where("organization_id = ? AND whats_app_account = ? AND direction = ? AND message_type = ?",
		orgID, event.WhatsAppAccount, models.DirectionIncoming, models.MessageType("button")).
		Find(&msgs).Error; err == nil {
		for i := range msgs {
			cid := msgs[i].ContactID
			if seen[cid] {
				continue
			}
			seen[cid] = true
			phone, name := contactInfo(cid)
			if phone == "" {
				continue
			}
			targets = append(targets, repromptTarget{ContactID: cid, Phone: phone, Name: name, Reason: "tapped, not recorded"})
		}
	}

	return targets, nil
}

// RepromptPreview returns the list of guests who would be re-prompted and the
// message they would receive, without sending anything.
func (a *App) RepromptPreview(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	eventID, err := parsePathUUID(r, "id", "RSVP event")
	if err != nil {
		return nil
	}
	event, err := findByIDAndOrg[models.RSVPEvent](a.DB, r, eventID, orgID, "RSVP event")
	if err != nil {
		return nil
	}
	if event.FlowID == nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "No flow is linked to this event", nil, "")
	}
	flow, err := a.getChatbotFlowByIDCached(orgID, *event.FlowID)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to load flow", nil, "")
	}
	targets, err := a.computeRepromptTargets(orgID, event)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to compute targets", nil, "")
	}
	message := flow.InitialMessage
	if firstStep := firstFlowStepMessage(flow); message == "" {
		message = firstStep
	}
	return r.SendEnvelope(map[string]interface{}{
		"targets": targets,
		"count":   len(targets),
		"message": message,
	})
}

func firstFlowStepMessage(flow *models.ChatbotFlow) string {
	if len(flow.Steps) > 0 {
		return flow.Steps[0].Message
	}
	return ""
}

// RepromptRSVPFlowSessions re-sends the RSVP flow to every incomplete guest,
// resetting their response to pending first so it re-runs cleanly and the
// duplicate guard doesn't turn them away.
func (a *App) RepromptRSVPFlowSessions(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	eventID, err := parsePathUUID(r, "id", "RSVP event")
	if err != nil {
		return nil
	}
	event, err := findByIDAndOrg[models.RSVPEvent](a.DB, r, eventID, orgID, "RSVP event")
	if err != nil {
		return nil
	}
	if event.FlowID == nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "No flow is linked to this event", nil, "")
	}
	flow, err := a.getChatbotFlowByIDCached(orgID, *event.FlowID)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to load flow", nil, "")
	}
	targets, err := a.computeRepromptTargets(orgID, event)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to compute targets", nil, "")
	}

	// Optional: restrict to a selected subset of phone numbers (matched by trailing
	// digits so format differences don't matter).
	var body struct {
		Phones []string `json:"phones"`
	}
	_ = json.Unmarshal(r.RequestCtx.PostBody(), &body)
	if len(body.Phones) > 0 {
		selected := map[string]bool{}
		for _, p := range body.Phones {
			if suffix := phoneMatchSuffix(p); suffix != "" {
				selected[suffix] = true
			}
		}
		filtered := targets[:0]
		for _, t := range targets {
			if selected[phoneMatchSuffix(t.Phone)] {
				filtered = append(filtered, t)
			}
		}
		targets = filtered
	}

	timeoutMins := 1440
	if settings, err := a.getChatbotSettingsCached(orgID, event.WhatsAppAccount); err == nil && settings.SessionTimeoutMins > 0 {
		timeoutMins = settings.SessionTimeoutMins
	}

	account, err := a.resolveWhatsAppAccount(orgID, event.WhatsAppAccount)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "WhatsApp account not found", nil, "")
	}

	reprompted := 0
	for _, tgt := range targets {
		var contact models.Contact
		if err := a.DB.Where("id = ? AND organization_id = ?", tgt.ContactID, orgID).First(&contact).Error; err != nil {
			continue
		}
		// Reset any existing response to pending so a re-run isn't blocked by the
		// duplicate guard and overwrites cleanly on completion.
		a.DB.Unscoped().Model(&models.RSVPResponse{}).
			Where("rsvp_event_id = ? AND contact_id = ?", event.ID, contact.ID).
			Updates(map[string]interface{}{"attendance": models.RSVPAttendancePending, "deleted_at": nil})
		// End any current session so the flow starts fresh.
		a.DB.Model(&models.ChatbotSession{}).
			Where("organization_id = ? AND contact_id = ? AND status = ?", orgID, contact.ID, models.SessionStatus("active")).
			Update("status", models.SessionStatus("completed"))

		session, _ := a.getOrCreateSession(orgID, contact.ID, account.Name, contact.PhoneNumber, timeoutMins)
		a.startFlow(account, session, &contact, flow, "", "")
		now := time.Now()
		a.DB.Model(&models.RSVPResponse{}).
			Where("rsvp_event_id = ? AND contact_id = ?", event.ID, contact.ID).
			Update("reprompted_at", now)
		reprompted++
	}

	return r.SendEnvelope(map[string]interface{}{
		"reprompted": reprompted,
		"message":    "Re-sent the RSVP flow to incomplete guests.",
	})
}
