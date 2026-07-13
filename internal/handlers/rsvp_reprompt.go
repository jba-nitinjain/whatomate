package handlers

import (
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// RepromptRSVPFlowSessions re-sends the current step's message to guests who are
// stuck mid-flow for an event (active sessions that never completed), so replies
// missed earlier can be answered and captured.
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

	var sessions []models.ChatbotSession
	if err := a.DB.Where("organization_id = ? AND current_flow_id = ? AND status = ?",
		orgID, *event.FlowID, models.SessionStatus("active")).Find(&sessions).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to load sessions", nil, "")
	}

	stepByName := make(map[string]*models.ChatbotFlowStep, len(flow.Steps))
	for i := range flow.Steps {
		stepByName[flow.Steps[i].StepName] = &flow.Steps[i]
	}

	reprompted := 0
	for i := range sessions {
		s := &sessions[i]
		if evID, _ := s.SessionData[rsvpEventIDKey].(string); evID != event.ID.String() {
			continue
		}
		step := stepByName[s.CurrentStep]
		if step == nil {
			continue
		}
		account, err := a.resolveWhatsAppAccount(orgID, s.WhatsAppAccount)
		if err != nil {
			continue
		}
		var contact models.Contact
		if err := a.DB.Where("id = ? AND organization_id = ?", s.ContactID, orgID).First(&contact).Error; err != nil {
			continue
		}
		a.sendStepMessage(account, s, &contact, step)
		reprompted++
	}

	return r.SendEnvelope(map[string]interface{}{
		"reprompted": reprompted,
		"message":    "Re-prompt sent to guests still in the flow.",
	})
}
