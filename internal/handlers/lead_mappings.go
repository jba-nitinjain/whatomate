package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/leadutil"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/internal/templateutil"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// validateLeadMapping checks that every reference on a mapping belongs to the
// caller's organization and that the template is usable. It returns an empty
// string when valid, otherwise a user-facing error message.
func (a *App) validateLeadMapping(orgID uuid.UUID, m *models.LeadFormMapping) string {
	if m.TemplateID != nil {
		var tmpl models.Template
		if err := a.DB.Where("id = ? AND organization_id = ?", *m.TemplateID, orgID).First(&tmpl).Error; err != nil {
			return "Template not found"
		}
		if tmpl.Status != string(models.TemplateStatusApproved) {
			return fmt.Sprintf("Template is not approved (status: %s)", tmpl.Status)
		}
		for _, v := range templateutil.ExtParamNames(tmpl.BodyContent) {
			if _, ok := m.TemplateParams[v]; !ok {
				return fmt.Sprintf("Variable %q is not mapped", v)
			}
		}
	}
	if m.WhatsAppAccount != "" {
		var n int64
		a.DB.Model(&models.WhatsAppAccount{}).Where("organization_id = ? AND name = ?", orgID, m.WhatsAppAccount).Count(&n)
		if n == 0 {
			return "WhatsApp account not found"
		}
	}
	if m.AssignUserID != nil {
		var n int64
		a.DB.Model(&models.UserOrganization{}).Where("organization_id = ? AND user_id = ?", orgID, *m.AssignUserID).Count(&n)
		if n == 0 {
			return "Assigned user not found"
		}
	}
	if m.AssignTeamID != nil {
		var n int64
		a.DB.Model(&models.Team{}).Where("organization_id = ? AND id = ?", orgID, *m.AssignTeamID).Count(&n)
		if n == 0 {
			return "Assigned team not found"
		}
	}
	return ""
}

// ListLeadMappings lists form mappings for a source
func (a *App) ListLeadMappings(r *fastglue.Request) error {
	orgID, _, ok := a.leadAuth(r, models.ActionRead)
	if !ok {
		return nil
	}
	sourceID, err := parsePathUUID(r, "id", "lead source")
	if err != nil {
		return nil
	}

	var mappings []models.LeadFormMapping
	if err := a.DB.Preload("Template").Where("lead_source_id = ? AND organization_id = ?", sourceID, orgID).Find(&mappings).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to list mappings", nil, "")
	}
	return r.SendEnvelope(mappings)
}

// CreateLeadMapping creates a mapping for a lead source owned by the caller's org
func (a *App) CreateLeadMapping(r *fastglue.Request) error {
	orgID, _, ok := a.leadAuth(r, models.ActionWrite)
	if !ok {
		return nil
	}
	source := a.loadLeadSource(r, orgID)
	if source == nil {
		return nil
	}

	var req models.LeadFormMapping
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	req.FormID = strings.TrimSpace(req.FormID)
	if req.FormID == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Form ID is required", nil, "")
	}
	if msg := a.validateLeadMapping(orgID, &req); msg != "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, msg, nil, "")
	}

	req.BaseModel = models.BaseModel{ID: uuid.New()}
	req.OrganizationID = orgID
	req.LeadSourceID = source.ID
	req.Template = nil
	if err := a.DB.Create(&req).Error; err != nil {
		if isUniqueViolation(err) {
			return r.SendErrorEnvelope(fasthttp.StatusConflict, "A mapping for this form already exists", nil, "")
		}
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to create mapping", nil, "")
	}
	return r.SendEnvelope(req)
}

// UpdateLeadMapping updates a mapping
func (a *App) UpdateLeadMapping(r *fastglue.Request) error {
	orgID, _, ok := a.leadAuth(r, models.ActionWrite)
	if !ok {
		return nil
	}
	mappingID, err := parsePathUUID(r, "id", "mapping")
	if err != nil {
		return nil
	}
	existing, err := findByIDAndOrg[models.LeadFormMapping](a.DB, r, mappingID, orgID, "Mapping")
	if err != nil {
		return nil
	}

	var req models.LeadFormMapping
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	if msg := a.validateLeadMapping(orgID, &req); msg != "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, msg, nil, "")
	}

	existing.FormName = req.FormName
	existing.IsActive = req.IsActive
	existing.WhatsAppAccount = req.WhatsAppAccount
	existing.TemplateID = req.TemplateID
	existing.TemplateParams = req.TemplateParams
	existing.HeaderParams = req.HeaderParams
	existing.ButtonParams = req.ButtonParams
	existing.HeaderMediaID = req.HeaderMediaID
	existing.PhoneField = req.PhoneField
	existing.NameField = req.NameField
	existing.ConsentField = req.ConsentField
	existing.Tags = req.Tags
	existing.AssignUserID = req.AssignUserID
	existing.AssignTeamID = req.AssignTeamID
	existing.SendDelaySeconds = req.SendDelaySeconds
	existing.DedupeHours = req.DedupeHours
	existing.SkipIfOptedOut = req.SkipIfOptedOut

	if err := a.DB.Save(existing).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to update mapping", nil, "")
	}
	return r.SendEnvelope(existing)
}

// DeleteLeadMapping deletes a mapping
func (a *App) DeleteLeadMapping(r *fastglue.Request) error {
	orgID, _, ok := a.leadAuth(r, models.ActionDelete)
	if !ok {
		return nil
	}
	mappingID, err := parsePathUUID(r, "id", "mapping")
	if err != nil {
		return nil
	}
	if err := a.DB.Where("id = ? AND organization_id = ?", mappingID, orgID).Delete(&models.LeadFormMapping{}).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to delete mapping", nil, "")
	}
	return r.SendEnvelope(map[string]bool{"success": true})
}

// PreviewLeadMapping renders the template message against sample data or an existing lead
func (a *App) PreviewLeadMapping(r *fastglue.Request) error {
	orgID, _, ok := a.leadAuth(r, models.ActionRead)
	if !ok {
		return nil
	}
	mappingID, err := parsePathUUID(r, "id", "mapping")
	if err != nil {
		return nil
	}

	var mapping models.LeadFormMapping
	if err := a.DB.Preload("Template").Where("id = ? AND organization_id = ?", mappingID, orgID).First(&mapping).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Mapping not found", nil, "")
	}
	if mapping.Template == nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Mapping has no template assigned", nil, "")
	}

	var req struct {
		LeadID string `json:"lead_id"`
	}
	_ = r.Decode(&req, "json") // body is optional

	var lead models.Lead
	if lID, err := uuid.Parse(req.LeadID); err == nil {
		_ = a.DB.Where("id = ? AND organization_id = ?", lID, orgID).First(&lead).Error
	}
	if lead.ID == uuid.Nil {
		lead = sampleLead(mapping.FormID)
	}

	tmplData := leadutil.BuildTemplateData(&lead, &mapping, "Sample Page")
	renderedBody := mapping.Template.BodyContent
	for k, expr := range mapping.TemplateParams {
		val := processTemplate(fmt.Sprintf("%v", expr), tmplData)
		renderedBody = strings.ReplaceAll(renderedBody, "{{"+k+"}}", leadutil.CleanParamValue(val))
	}

	return r.SendEnvelope(map[string]interface{}{
		"template_name": mapping.Template.Name,
		"language":      mapping.Template.Language,
		"rendered_body": renderedBody,
	})
}

// sampleLead returns placeholder data for previews when no real lead is chosen.
func sampleLead(formID string) models.Lead {
	return models.Lead{
		Name:          "John Doe",
		Phone:         "919876543210",
		Email:         "john@example.com",
		FormID:        formID,
		AdName:        "Summer Campaign Ad #1",
		CampaignName:  "Lead Generation 2026",
		LeadCreatedAt: time.Now(),
		FieldData: models.JSONB{
			"full_name":    "John Doe",
			"phone_number": "+91 98765 43210",
			"email":        "john@example.com",
		},
	}
}
