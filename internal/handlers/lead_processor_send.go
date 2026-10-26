package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/leadutil"
	"github.com/nikyjain/whatomate/internal/models"
	"gorm.io/gorm"
)

// leadSendOptions are the message options for Lead Ads auto-responses. The
// send is synchronous so the lead's status reflects the WhatsApp outcome.
func leadSendOptions() MessageSendOptions {
	return MessageSendOptions{
		BroadcastWebSocket: true,
		DispatchWebhook:    true,
		TrackSLA:           false,
		Async:              false,
	}
}

// applyLeadToContact merges mapping tags, metadata.lead, the WhatsApp account
// and the assignee into the contact with a single targeted UPDATE.
func (p *LeadProcessor) applyLeadToContact(contact *models.Contact, lead *models.Lead, mapping *models.LeadFormMapping, source *models.LeadSource) error {
	leadMeta, err := json.Marshal(map[string]interface{}{
		"source":        source.Name,
		"form_id":       lead.FormID,
		"form_name":     mapping.FormName,
		"ad_name":       lead.AdName,
		"campaign_name": lead.CampaignName,
		"leadgen_id":    lead.LeadgenID,
		"answers":       lead.FieldData,
	})
	if err != nil {
		return err
	}
	updates := map[string]interface{}{
		"metadata": gorm.Expr("COALESCE(metadata, '{}'::jsonb) || jsonb_build_object('lead', ?::jsonb)", string(leadMeta)),
	}

	if tags, changed := mergeContactTags(contact.Tags, mapping.Tags); changed {
		updates["tags"] = tags
		contact.Tags = tags
	}
	if contact.WhatsAppAccount == "" && mapping.WhatsAppAccount != "" {
		updates["whats_app_account"] = mapping.WhatsAppAccount
		contact.WhatsAppAccount = mapping.WhatsAppAccount
	}
	if contact.AssignedUserID == nil {
		var assignee *uuid.UUID
		if mapping.AssignUserID != nil {
			assignee = mapping.AssignUserID
		} else if mapping.AssignTeamID != nil && p.app.Assigner != nil {
			assignee = p.app.Assigner.AssignToTeam(*mapping.AssignTeamID, lead.OrganizationID, nil, nil)
		}
		if assignee != nil {
			updates["assigned_user_id"] = *assignee
			contact.AssignedUserID = assignee
		}
	}

	return p.app.DB.Model(&models.Contact{}).
		Where("id = ? AND organization_id = ?", contact.ID, lead.OrganizationID).
		Updates(updates).Error
}

// mergeContactTags appends mapping tags missing from the contact, keeping order.
func mergeContactTags(existing models.JSONBArray, add models.StringArray) (models.JSONBArray, bool) {
	seen := make(map[string]bool, len(existing))
	merged := make(models.JSONBArray, 0, len(existing)+len(add))
	for _, t := range existing {
		if s, ok := t.(string); ok {
			seen[s] = true
		}
		merged = append(merged, t)
	}
	changed := false
	for _, t := range add {
		if t != "" && !seen[t] {
			seen[t] = true
			merged = append(merged, t)
			changed = true
		}
	}
	return merged, changed
}

// sendTemplate renders the mapped template and sends it at most once.
func (p *LeadProcessor) sendTemplate(ctx context.Context, lead *models.Lead, source *models.LeadSource, mapping *models.LeadFormMapping, contact *models.Contact, tmpl *models.Template) {
	// A previous attempt already produced a message that did not fail: never
	// send a second one, just record the lead as sent.
	if lead.MessageID != nil {
		if status := p.messageStatus(*lead.MessageID); status != "" && status != models.MessageStatusFailed {
			p.markSendingThenSent(lead, *lead.MessageID)
			return
		}
	}

	bodyParams, buttonParams, missing := renderLeadParams(lead, mapping, source)
	if missing != "" {
		p.finish(lead, models.StatusLeadFailed, fmt.Sprintf("Missing parameter value for %q", missing))
		return
	}

	accountName := mapping.WhatsAppAccount
	if accountName == "" {
		accountName = tmpl.WhatsAppAccount
	}
	account, err := p.app.resolveWhatsAppAccount(lead.OrganizationID, accountName)
	if err != nil {
		p.finish(lead, models.StatusLeadFailed, "No active WhatsApp account found to send template")
		return
	}

	// Persist the "sending" marker atomically; if another worker got here first
	// (or the lead changed), stop without sending.
	if !p.markSending(lead) {
		return
	}

	msg, err := p.app.SendOutgoingMessage(ctx, OutgoingMessageRequest{
		Account:       account,
		Contact:       contact,
		Type:          models.MessageTypeTemplate,
		Template:      tmpl,
		BodyParams:    bodyParams,
		ButtonParams:  buttonParams,
		HeaderMediaID: mapping.HeaderMediaID,
	}, leadSendOptions())
	if err == nil {
		err = p.messageSendError(msg.ID)
	}
	if err != nil {
		p.handleSendFailure(lead, contact, msg, err)
		return
	}
	p.completeSend(lead, msg.ID)
}

// renderLeadParams evaluates the mapping expressions against the lead. It
// returns the name of the first body parameter that rendered empty, if any.
func renderLeadParams(lead *models.Lead, mapping *models.LeadFormMapping, source *models.LeadSource) (map[string]string, map[int]string, string) {
	data := leadutil.BuildTemplateData(lead, mapping, source.PageName)
	render := func(expr interface{}) string {
		return leadutil.CleanParamValue(processTemplate(fmt.Sprintf("%v", expr), data))
	}

	body := make(map[string]string, len(mapping.TemplateParams))
	for k, expr := range mapping.TemplateParams {
		v := render(expr)
		if v == "" {
			return nil, nil, k
		}
		body[k] = v
	}
	for k, expr := range mapping.HeaderParams {
		if v := render(expr); v != "" {
			body[k] = v
		}
	}
	buttons := make(map[int]string, len(mapping.ButtonParams))
	for k, expr := range mapping.ButtonParams {
		if idx, err := strconv.Atoi(k); err == nil {
			buttons[idx] = render(expr)
		}
	}
	return body, buttons, ""
}

// markSending moves a claimable lead to "sending", persisting the work done so
// far. It returns false when the row is no longer claimable.
func (p *LeadProcessor) markSending(lead *models.Lead) bool {
	res := p.app.DB.Model(&models.Lead{}).
		Where("id = ? AND status IN ? AND deleted_at IS NULL", lead.ID, claimableLeadStatuses).
		Updates(map[string]interface{}{
			"status":     models.StatusLeadSending,
			"last_error": "",
			"mapping_id": lead.MappingID,
			"name":       lead.Name,
			"phone_raw":  lead.PhoneRaw,
			"phone":      lead.Phone,
			"email":      lead.Email,
			"contact_id": lead.ContactID,
		})
	if res.Error != nil {
		p.app.Log.Error("Failed to mark lead as sending", "lead_id", lead.ID, "error", res.Error)
		return false
	}
	if res.RowsAffected != 1 {
		p.app.Log.Warn("Lead no longer claimable; skipping send", "lead_id", lead.ID)
		return false
	}
	lead.Status = models.StatusLeadSending
	return true
}

// markSendingThenSent records an already-delivered message as the lead's send.
func (p *LeadProcessor) markSendingThenSent(lead *models.Lead, messageID uuid.UUID) {
	if p.markSending(lead) {
		p.completeSend(lead, messageID)
	}
}

// completeSend finalises a successful send (only from the "sending" state).
func (p *LeadProcessor) completeSend(lead *models.Lead, messageID uuid.UUID) {
	now := time.Now()
	lead.Status = models.StatusLeadSent
	lead.MessageID = &messageID
	lead.SentAt = &now
	lead.LastError = ""
	p.updateFromSending(lead, map[string]interface{}{
		"status": lead.Status, "message_id": messageID, "sent_at": now, "last_error": "",
	})
	p.app.DispatchWebhook(lead.OrganizationID, models.WebhookEventLeadSent, map[string]interface{}{
		"id":         lead.ID,
		"leadgen_id": lead.LeadgenID,
		"phone":      lead.Phone,
		"message_id": messageID,
		"sent_at":    now,
	})
	p.broadcastLeadUpdate(lead)
}

// handleSendFailure classifies a failed send and moves the lead out of "sending".
func (p *LeadProcessor) handleSendFailure(lead *models.Lead, contact *models.Contact, msg *models.Message, sendErr error) {
	retryable, reason := leadutil.Classify(sendErr)
	status := models.StatusLeadFailed
	if code, _, _ := leadutil.MetaErrorCode(sendErr); code == leadutil.CodeMarketingOptOut {
		if err := p.app.DB.Model(&models.Contact{}).Where("id = ?", contact.ID).Update("marketing_opt_out", true).Error; err != nil {
			p.app.Log.Error("Failed to flag contact marketing opt-out", "contact_id", contact.ID, "error", err)
		}
		status = models.StatusLeadSkippedOptedOut
		retryable = false
	}

	updates := map[string]interface{}{"last_error": reason}
	if msg != nil {
		lead.MessageID = &msg.ID // failed message, kept for visibility; resend is allowed
		updates["message_id"] = msg.ID
	}
	if retryable && lead.Attempts < leadMaxAttempts {
		status = models.StatusLeadRetry
		next := time.Now().Add(backoffDuration(lead.Attempts))
		lead.NextAttemptAt = &next
		updates["next_attempt_at"] = next
	}
	lead.Status = status
	lead.LastError = reason
	updates["status"] = status
	p.updateFromSending(lead, updates)
	p.broadcastLeadUpdate(lead)
}

func (p *LeadProcessor) updateFromSending(lead *models.Lead, updates map[string]interface{}) {
	if err := p.app.DB.Model(&models.Lead{}).
		Where("id = ? AND status = ?", lead.ID, models.StatusLeadSending).
		Updates(updates).Error; err != nil {
		p.app.Log.Error("Failed to finalise lead send", "lead_id", lead.ID, "error", err)
	}
}

// messageStatus returns the stored status of a message ("" if not found).
func (p *LeadProcessor) messageStatus(id uuid.UUID) models.MessageStatus {
	var msg models.Message
	if err := p.app.DB.Select("status").Where("id = ?", id).First(&msg).Error; err != nil {
		return ""
	}
	return msg.Status
}

// messageSendError turns a synchronously failed send into an error.
// SendOutgoingMessage records WhatsApp failures on the message row instead of
// returning them, so the row is the source of truth.
func (p *LeadProcessor) messageSendError(id uuid.UUID) error {
	var msg models.Message
	if err := p.app.DB.Select("status", "error_message").Where("id = ?", id).First(&msg).Error; err != nil {
		return fmt.Errorf("failed to read message status: %w", err)
	}
	if msg.Status == models.MessageStatusFailed {
		if msg.ErrorMessage == "" {
			return errors.New("message send failed")
		}
		return errors.New(msg.ErrorMessage)
	}
	return nil
}
