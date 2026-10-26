package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nikyjain/whatomate/internal/contactutil"
	"github.com/nikyjain/whatomate/internal/leadutil"
	"github.com/nikyjain/whatomate/internal/models"
)

// processLead runs one claimed lead through the pipeline. Each stage either
// advances the lead or records its outcome and stops the pipeline.
func (p *LeadProcessor) processLead(ctx context.Context, lead *models.Lead) {
	source := p.loadSource(lead)
	if source == nil {
		return
	}
	if !p.fetchDetails(ctx, lead, source) {
		return
	}
	mapping := p.resolveMapping(lead, source)
	if mapping == nil {
		return
	}
	if !p.extractFields(lead, mapping, source) {
		return
	}
	contact := p.upsertContact(lead, mapping, source)
	if contact == nil {
		return
	}
	if p.isDuplicate(lead, mapping) {
		return
	}
	if mapping.TemplateID == nil {
		p.finish(lead, models.StatusLeadStored, "")
		return
	}
	tmpl := p.loadTemplate(lead, mapping)
	if tmpl == nil {
		return
	}
	if p.isOptedOut(lead, mapping, tmpl, contact) || p.deferForDelay(lead, mapping) {
		return
	}
	p.sendTemplate(ctx, lead, source, mapping, contact, tmpl)
}

// loadSource loads the lead's org-scoped source and rejects inactive ones.
func (p *LeadProcessor) loadSource(lead *models.Lead) *models.LeadSource {
	var source models.LeadSource
	if err := p.app.DB.Where("id = ? AND organization_id = ?", lead.LeadSourceID, lead.OrganizationID).First(&source).Error; err != nil {
		p.finish(lead, models.StatusLeadFailed, fmt.Sprintf("Lead source not found: %v", err))
		return nil
	}
	if !source.IsActive {
		p.finish(lead, models.StatusLeadFailed, "Lead source is inactive")
		return nil
	}
	source.DecryptSecrets(p.app.Config.App.EncryptionKey)
	return &source
}

// fetchDetails pulls the lead's answers from the Graph API whenever they have
// not been fetched yet, independent of whether this is a first try or a retry.
func (p *LeadProcessor) fetchDetails(ctx context.Context, lead *models.Lead, source *models.LeadSource) bool {
	needsFetch := len(lead.FieldData) == 0
	if needsFetch {
		details, err := p.app.WhatsApp.GetLead(ctx, source.APIVersion, lead.LeadgenID, source.PageAccessToken)
		if err != nil {
			retryable, reason := leadutil.Classify(err)
			if code, _, _ := leadutil.MetaErrorCode(err); code == leadutil.CodeTokenInvalid {
				p.deactivateSource(source)
			}
			p.failOrRetry(lead, retryable, reason)
			return false
		}
		lead.AdID = details.AdID
		lead.AdName = details.AdName
		lead.CampaignName = details.CampaignName
		lead.Platform = details.Platform
		lead.IsOrganic = details.IsOrganic
		if details.FormID != "" {
			lead.FormID = details.FormID
		}
		lead.FieldData = models.JSONB(leadutil.FlattenFieldData(details.FieldData))
	}

	// First time the answers are available (fetched now, or backfilled by sync).
	if needsFetch || lead.Status == models.StatusLeadReceived {
		lead.Status = models.StatusLeadFetched
		p.saveLead(lead)
		p.dispatchLeadReceived(lead)
		p.broadcastLeadUpdate(lead)
	}
	return true
}

// deactivateSource disables a source whose Page token Meta has revoked.
func (p *LeadProcessor) deactivateSource(source *models.LeadSource) {
	if err := p.app.DB.Model(&models.LeadSource{}).Where("id = ?", source.ID).Update("is_active", false).Error; err != nil {
		p.app.Log.Error("Failed to deactivate lead source", "source_id", source.ID, "error", err)
	}
	p.app.InvalidateLeadSourceCache(source.PageID)
}

func (p *LeadProcessor) dispatchLeadReceived(lead *models.Lead) {
	payload := map[string]interface{}{
		"id":            lead.ID,
		"leadgen_id":    lead.LeadgenID,
		"page_id":       lead.PageID,
		"form_id":       lead.FormID,
		"ad_id":         lead.AdID,
		"ad_name":       lead.AdName,
		"campaign_name": lead.CampaignName,
		"platform":      lead.Platform,
		"is_organic":    lead.IsOrganic,
		"created_at":    lead.LeadCreatedAt,
	}
	if !p.leadAnswersRedacted() {
		payload["field_data"] = lead.FieldData
	}
	p.app.DispatchWebhook(lead.OrganizationID, models.WebhookEventLeadReceived, payload)
}

// leadAnswersRedacted reports whether the "redact_lead_answers" system setting
// ({"enabled": true}) asks to keep form answers out of outbound webhooks.
func (p *LeadProcessor) leadAnswersRedacted() bool {
	var setting models.SystemSetting
	if err := p.app.DB.Where("key = ?", "redact_lead_answers").First(&setting).Error; err != nil {
		return false
	}
	enabled, _ := setting.Value["enabled"].(bool)
	return enabled
}

// resolveMapping finds the active mapping for the form, falling back to the
// page default ("*"). Lookups are scoped to the lead's organization.
func (p *LeadProcessor) resolveMapping(lead *models.Lead, source *models.LeadSource) *models.LeadFormMapping {
	var mapping models.LeadFormMapping
	find := func(formID string) error {
		return p.app.DB.Where("organization_id = ? AND lead_source_id = ? AND is_active = ? AND form_id = ?",
			lead.OrganizationID, source.ID, true, formID).First(&mapping).Error
	}
	if err := find(lead.FormID); err != nil {
		if err := find("*"); err != nil {
			p.finish(lead, models.StatusLeadSkippedNoMapping, fmt.Sprintf("No active mapping for form %s or default *", lead.FormID))
			return nil
		}
	}
	lead.MappingID = &mapping.ID
	if mapping.FormName == "" && lead.FormID != "*" {
		mapping.FormName = lead.FormID
	}
	return &mapping
}

// extractFields reads name/phone/email from the answers, normalises the phone
// and enforces the consent field when one is configured.
func (p *LeadProcessor) extractFields(lead *models.Lead, mapping *models.LeadFormMapping, source *models.LeadSource) bool {
	phoneField := mapping.PhoneField
	if phoneField == "" {
		phoneField = "phone_number"
	}
	nameField := mapping.NameField
	if nameField == "" {
		nameField = "full_name"
	}

	lead.PhoneRaw = fieldString(lead.FieldData, phoneField)
	lead.Name = fieldString(lead.FieldData, nameField)
	if lead.Name == "" {
		lead.Name = strings.TrimSpace(fieldString(lead.FieldData, "first_name") + " " + fieldString(lead.FieldData, "last_name"))
	}
	lead.Email = fieldString(lead.FieldData, "email")

	phone, err := leadutil.NormalizePhone(lead.PhoneRaw, source.DefaultCountryCode)
	if err != nil {
		p.finish(lead, models.StatusLeadSkippedInvalidPhone, fmt.Sprintf("Invalid phone %q: %v", lead.PhoneRaw, err))
		return false
	}
	lead.Phone = phone

	if mapping.ConsentField != "" {
		consent := fieldString(lead.FieldData, mapping.ConsentField)
		if !leadutil.IsAffirmativeConsent(consent) {
			p.finish(lead, models.StatusLeadSkippedNoConsent, fmt.Sprintf("Consent field %q value %q not affirmative", mapping.ConsentField, consent))
			return false
		}
	}
	return true
}

// fieldString returns a form answer as a string ("" when missing or nil).
func fieldString(data models.JSONB, key string) string {
	if v, ok := data[key]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// isDuplicate skips the lead when the same phone+form was messaged recently.
func (p *LeadProcessor) isDuplicate(lead *models.Lead, mapping *models.LeadFormMapping) bool {
	if mapping.DedupeHours <= 0 {
		return false
	}
	cutoff := time.Now().Add(-time.Duration(mapping.DedupeHours) * time.Hour)
	var n int64
	p.app.DB.Model(&models.Lead{}).
		Where("organization_id = ? AND phone = ? AND form_id = ? AND status IN ? AND sent_at > ? AND id <> ?",
			lead.OrganizationID, lead.Phone, lead.FormID,
			[]string{models.StatusLeadSent, models.StatusLeadSending}, cutoff, lead.ID).
		Count(&n)
	if n == 0 {
		return false
	}
	p.finish(lead, models.StatusLeadSkippedDuplicate,
		fmt.Sprintf("Duplicate lead for phone %s and form %s within %d hours", lead.Phone, lead.FormID, mapping.DedupeHours))
	return true
}

// loadTemplate loads the mapped, org-scoped template and requires it approved.
func (p *LeadProcessor) loadTemplate(lead *models.Lead, mapping *models.LeadFormMapping) *models.Template {
	var tmpl models.Template
	if err := p.app.DB.Where("id = ? AND organization_id = ?", *mapping.TemplateID, lead.OrganizationID).First(&tmpl).Error; err != nil {
		p.finish(lead, models.StatusLeadFailed, fmt.Sprintf("Template %v not found: %v", *mapping.TemplateID, err))
		return nil
	}
	if tmpl.Status != string(models.TemplateStatusApproved) {
		p.finish(lead, models.StatusLeadFailed, fmt.Sprintf("Template %s is not approved (status: %s)", tmpl.Name, tmpl.Status))
		return nil
	}
	return &tmpl
}

// isOptedOut skips marketing templates for contacts who opted out of marketing.
func (p *LeadProcessor) isOptedOut(lead *models.Lead, mapping *models.LeadFormMapping, tmpl *models.Template, contact *models.Contact) bool {
	if tmpl.Category != string(models.TemplateCategoryMarketing) || !contact.MarketingOptOut || !mapping.SkipIfOptedOut {
		return false
	}
	p.finish(lead, models.StatusLeadSkippedOptedOut, "Recipient has opted out of marketing templates")
	return true
}

// deferForDelay postpones the send until the mapping's delay has elapsed.
func (p *LeadProcessor) deferForDelay(lead *models.Lead, mapping *models.LeadFormMapping) bool {
	if mapping.SendDelaySeconds <= 0 {
		return false
	}
	delayUntil := lead.LeadCreatedAt.Add(time.Duration(mapping.SendDelaySeconds) * time.Second)
	if !time.Now().Before(delayUntil) {
		return false
	}
	lead.NextAttemptAt = &delayUntil
	if lead.Attempts > 0 {
		lead.Attempts-- // waiting is not a failed attempt
	}
	p.saveLead(lead)
	return true
}

// upsertContact creates or updates the contact for the lead using targeted
// column updates so concurrent edits to unrelated contact fields survive.
func (p *LeadProcessor) upsertContact(lead *models.Lead, mapping *models.LeadFormMapping, source *models.LeadSource) *models.Contact {
	contact, isNew, err := contactutil.GetOrCreateContact(p.app.DB, lead.OrganizationID, lead.Phone, lead.Name)
	if err != nil {
		p.finish(lead, models.StatusLeadFailed, fmt.Sprintf("Failed to get or create contact: %v", err))
		return nil
	}
	lead.ContactID = &contact.ID

	if err := p.applyLeadToContact(contact, lead, mapping, source); err != nil {
		p.app.Log.Error("Failed to update contact from lead", "lead_id", lead.ID, "contact_id", contact.ID, "error", err)
	}
	if isNew {
		p.app.DispatchWebhook(lead.OrganizationID, models.WebhookEventContactCreated, contact)
	}
	return contact
}
