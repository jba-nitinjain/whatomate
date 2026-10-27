package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/leadutil"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/pkg/whatsapp"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm/clause"
)

// SubscribeLeadSource calls POST /{page-id}/subscribed_apps?subscribed_fields=leadgen
func (a *App) SubscribeLeadSource(r *fastglue.Request) error {
	orgID, _, ok := a.leadAuth(r, models.ActionWrite)
	if !ok {
		return nil
	}
	source := a.loadLeadSource(r, orgID)
	if source == nil {
		return nil
	}
	source.DecryptSecrets(a.Config.App.EncryptionKey)

	ctx, cancel := context.WithTimeout(r.RequestCtx, 10*time.Second)
	defer cancel()
	if err := a.WhatsApp.SubscribePage(ctx, source.APIVersion, source.PageID, source.PageAccessToken); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, fmt.Sprintf("Failed to subscribe page app: %v", err), nil, "")
	}

	if err := a.DB.Model(&models.LeadSource{}).Where("id = ? AND organization_id = ?", source.ID, orgID).
		Update("subscribed", true).Error; err != nil {
		a.Log.Error("Failed to persist lead source subscription", "source_id", source.ID, "error", err)
	}
	a.InvalidateLeadSourceCache(source.PageID)
	return r.SendEnvelope(map[string]bool{"subscribed": true})
}

// TestLeadSourceConnection checks token validity, leads access, and app subscription
func (a *App) TestLeadSourceConnection(r *fastglue.Request) error {
	orgID, _, ok := a.leadAuth(r, models.ActionWrite)
	if !ok {
		return nil
	}
	source := a.loadLeadSource(r, orgID)
	if source == nil {
		return nil
	}
	source.DecryptSecrets(a.Config.App.EncryptionKey)

	ctx, cancel := context.WithTimeout(r.RequestCtx, 15*time.Second)
	defer cancel()

	checklist := map[string]interface{}{}
	if page, err := a.WhatsApp.GetPage(ctx, source.APIVersion, source.PageID, source.PageAccessToken); err != nil {
		checklist["token_valid"] = false
		checklist["token_error"] = err.Error()
	} else {
		checklist["token_valid"] = true
		checklist["page_name"] = page.Name
	}

	if forms, err := a.WhatsApp.GetPageForms(ctx, source.APIVersion, source.PageID, source.PageAccessToken); err != nil {
		checklist["leads_access"] = false
		checklist["leads_access_error"] = err.Error()
	} else {
		checklist["leads_access"] = true
		checklist["forms_count"] = len(forms)
	}

	checklist["subscribed"] = source.Subscribed
	checklist["app_secret_set"] = source.AppSecret != ""
	return r.SendEnvelope(checklist)
}

// leadFormItem is a Meta form merged with its Whatomate mapping.
type leadFormItem struct {
	ID         string                      `json:"id"`
	Name       string                      `json:"name"`
	Status     string                      `json:"status"`
	Questions  []whatsapp.LeadFormQuestion `json:"questions"`
	LeadsCount int                         `json:"leads_count"`
	Mapping    *models.LeadFormMapping     `json:"mapping,omitempty"`
}

// GetLeadSourceForms lists Meta forms merged with Whatomate mappings
func (a *App) GetLeadSourceForms(r *fastglue.Request) error {
	orgID, _, ok := a.leadAuth(r, models.ActionRead)
	if !ok {
		return nil
	}
	source := a.loadLeadSource(r, orgID)
	if source == nil {
		return nil
	}
	source.DecryptSecrets(a.Config.App.EncryptionKey)

	ctx, cancel := context.WithTimeout(r.RequestCtx, 10*time.Second)
	defer cancel()
	metaForms, err := a.WhatsApp.GetPageForms(ctx, source.APIVersion, source.PageID, source.PageAccessToken)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, fmt.Sprintf("Failed to fetch forms: %v", err), nil, "")
	}

	var mappings []models.LeadFormMapping
	if err := a.DB.Where("lead_source_id = ? AND organization_id = ?", source.ID, orgID).Find(&mappings).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to load mappings", nil, "")
	}
	mappingByForm := make(map[string]models.LeadFormMapping, len(mappings))
	for _, m := range mappings {
		mappingByForm[m.FormID] = m
	}

	result := make([]leadFormItem, len(metaForms))
	for i, f := range metaForms {
		item := leadFormItem{ID: f.ID, Name: f.Name, Status: f.Status, Questions: f.Questions, LeadsCount: f.LeadsCount}
		if m, found := mappingByForm[f.ID]; found {
			mapped := m
			item.Mapping = &mapped
		}
		result[i] = item
	}
	return r.SendEnvelope(result)
}

// leadSyncRequest is the body of POST /api/lead-sources/{id}/sync
type leadSyncRequest struct {
	FormID string `json:"form_id"`
	Since  string `json:"since"`
	Send   bool   `json:"send"` // default false: backfilled leads are stored only
}

// SyncLeadSource performs backfill of leads from form(s)
func (a *App) SyncLeadSource(r *fastglue.Request) error {
	orgID, _, ok := a.leadAuth(r, models.ActionExecute)
	if !ok {
		return nil
	}
	source := a.loadLeadSource(r, orgID)
	if source == nil {
		return nil
	}

	var req leadSyncRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	var sinceUnix int64
	if req.Since != "" {
		if t, err := time.Parse(time.RFC3339, req.Since); err == nil {
			sinceUnix = t.Unix()
		}
	}
	source.DecryptSecrets(a.Config.App.EncryptionKey)

	formIDs := a.leadSyncFormIDs(source, orgID, req.FormID)
	if len(formIDs) == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "No form IDs found to sync", nil, "")
	}

	ctx, cancel := context.WithTimeout(r.RequestCtx, 60*time.Second)
	defer cancel()

	totalSynced := 0
	now := time.Now()
	for _, formID := range formIDs {
		resp, err := a.WhatsApp.GetFormLeads(ctx, source.APIVersion, formID, source.PageAccessToken, sinceUnix, 100)
		if err != nil {
			a.Log.Error("Failed to fetch form leads for sync", "form_id", formID, "error", err)
			continue
		}
		for _, item := range resp.Data {
			if a.insertSyncedLead(source, orgID, formID, item, req.Send, now) {
				totalSynced++
			}
		}
	}

	if err := a.DB.Model(&models.LeadSource{}).Where("id = ?", source.ID).Update("last_synced_at", now).Error; err != nil {
		a.Log.Error("Failed to update lead source last_synced_at", "source_id", source.ID, "error", err)
	}
	if req.Send && totalSynced > 0 && a.LeadProcessor != nil {
		a.LeadProcessor.Kick()
	}
	return r.SendEnvelope(map[string]interface{}{"synced": totalSynced})
}

// leadSyncFormIDs returns the explicit form or all active, non-wildcard mapped forms.
func (a *App) leadSyncFormIDs(source *models.LeadSource, orgID uuid.UUID, formID string) []string {
	if formID != "" {
		return []string{formID}
	}
	var mappings []models.LeadFormMapping
	a.DB.Where("lead_source_id = ? AND organization_id = ? AND is_active = ?", source.ID, orgID, true).Find(&mappings)
	var ids []string
	for _, m := range mappings {
		if m.FormID != "*" {
			ids = append(ids, m.FormID)
		}
	}
	return ids
}

// insertSyncedLead idempotently inserts one backfilled lead and reports whether a row was created.
func (a *App) insertSyncedLead(source *models.LeadSource, orgID uuid.UUID, formID string, item whatsapp.LeadDetails, send bool, now time.Time) bool {
	leadCreatedAt, _ := time.Parse(time.RFC3339, item.CreatedTime)
	if leadCreatedAt.IsZero() {
		leadCreatedAt = now
	}
	status := models.StatusLeadReceived
	if !send {
		status = models.StatusLeadStored // don't message historical leads
	}

	lead := models.Lead{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: orgID,
		LeadSourceID:   source.ID,
		LeadgenID:      item.ID,
		PageID:         source.PageID,
		FormID:         formID,
		AdID:           item.AdID,
		AdName:         item.AdName,
		CampaignName:   item.CampaignName,
		Platform:       item.Platform,
		IsOrganic:      item.IsOrganic,
		FieldData:      models.JSONB(leadutil.FlattenFieldData(item.FieldData)),
		Status:         status,
		NextAttemptAt:  &now,
		LeadCreatedAt:  leadCreatedAt,
	}
	res := a.DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "leadgen_id"}}, DoNothing: true}).Create(&lead)
	if res.Error != nil {
		a.Log.Error("Failed to insert synced lead", "leadgen_id", item.ID, "error", res.Error)
		return false
	}
	return res.RowsAffected > 0
}
