package handlers

import (
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// RepromptRSVPFlowSessions re-runs the RSVP flow for guests whose response is
// still pending — i.e. they started the flow but never recorded a valid
// attendance (stuck mid-flow, or completed the flow without their answer being
// captured). Pending rows don't trip the duplicate guard, so the flow restarts
// cleanly and their answers are captured this time.
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

	var pending []models.RSVPResponse
	if err := a.DB.Where("organization_id = ? AND rsvp_event_id = ? AND attendance = ?",
		orgID, event.ID, models.RSVPAttendancePending).Find(&pending).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to load responses", nil, "")
	}

	timeoutMins := 1440
	if settings, err := a.getChatbotSettingsCached(orgID, event.WhatsAppAccount); err == nil && settings.SessionTimeoutMins > 0 {
		timeoutMins = settings.SessionTimeoutMins
	}

	reprompted := 0
	for i := range pending {
		resp := &pending[i]
		account, err := a.resolveWhatsAppAccount(orgID, event.WhatsAppAccount)
		if err != nil {
			continue
		}
		var contact models.Contact
		if err := a.DB.Where("id = ? AND organization_id = ?", resp.ContactID, orgID).First(&contact).Error; err != nil {
			continue
		}
		session, _ := a.getOrCreateSession(orgID, contact.ID, account.Name, contact.PhoneNumber, timeoutMins)
		a.startFlow(account, session, &contact, flow, "", "")
		reprompted++
	}

	return r.SendEnvelope(map[string]interface{}{
		"reprompted": reprompted,
		"message":    "Re-sent the RSVP flow to guests with a pending response.",
	})
}
