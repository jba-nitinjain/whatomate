package handlers

import (
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"

	"github.com/nikyjain/whatomate/internal/models"
)

// ReprocessMessageFlow re-runs an already-received message through the chatbot
// flow engine. It clears any active agent transfer for the contact (which would
// otherwise skip chatbot processing) and then either continues an in-progress
// flow or matches a flow trigger by the message text.
func (a *App) ReprocessMessageFlow(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	msgID, err := parsePathUUID(r, "id", "message")
	if err != nil {
		return nil
	}

	var msg models.Message
	if err := a.DB.Where("id = ? AND organization_id = ?", msgID, orgID).First(&msg).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Message not found", nil, "")
	}
	if msg.Direction != models.DirectionIncoming {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Only received messages can be reprocessed", nil, "")
	}

	var account models.WhatsAppAccount
	if err := a.DB.Where("organization_id = ? AND name = ?", orgID, msg.WhatsAppAccount).First(&account).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "WhatsApp account not found", nil, "")
	}
	var contact models.Contact
	if err := a.DB.Where("id = ? AND organization_id = ?", msg.ContactID, orgID).First(&contact).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Contact not found", nil, "")
	}

	// Clear any active agent transfer that would short-circuit chatbot processing.
	a.DB.Model(&models.AgentTransfer{}).
		Where("organization_id = ? AND contact_id = ? AND status = ?", orgID, contact.ID, models.TransferStatusActive).
		Update("status", models.TransferStatusResumed)

	a.runFlowEntryForMessage(&account, &contact, msg.Content)
	return r.SendEnvelope(map[string]interface{}{"reprocessed": true})
}

// runFlowEntryForMessage continues an active flow or starts a matching flow for
// the given contact using messageText, mirroring the incoming-message path.
func (a *App) runFlowEntryForMessage(account *models.WhatsAppAccount, contact *models.Contact, messageText string) {
	timeoutMins := 30
	if settings, err := a.getChatbotSettingsCached(account.OrganizationID, account.Name); err == nil && settings.SessionTimeoutMins > 0 {
		timeoutMins = settings.SessionTimeoutMins
	}
	session, _ := a.getOrCreateSession(account.OrganizationID, contact.ID, account.Name, contact.PhoneNumber, timeoutMins)
	if session.CurrentFlowID != nil {
		a.processFlowResponse(account, session, contact, messageText, "", nil)
		return
	}
	if flow := a.matchFlowTrigger(account.OrganizationID, account.Name, messageText); flow != nil {
		a.startFlow(account, session, contact, flow, messageText, "")
	}
}
