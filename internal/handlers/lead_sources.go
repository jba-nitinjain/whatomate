package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

// LeadSourceCreateRequest defines input for creating a connected Page
type LeadSourceCreateRequest struct {
	Name               string `json:"name"`
	PageID             string `json:"page_id"`
	PageAccessToken    string `json:"page_access_token"`
	AppID              string `json:"app_id"`
	AppSecret          string `json:"app_secret"`
	DefaultCountryCode string `json:"default_country_code"`
	APIVersion         string `json:"api_version"`
}

// LeadSourceUpdateRequest defines input for updating a connected Page
type LeadSourceUpdateRequest struct {
	Name               string `json:"name"`
	PageAccessToken    string `json:"page_access_token"` // blank = keep existing
	AppID              string `json:"app_id"`
	AppSecret          string `json:"app_secret"` // blank = keep existing; cannot be cleared
	DefaultCountryCode string `json:"default_country_code"`
	APIVersion         string `json:"api_version"`
	IsActive           *bool  `json:"is_active"`
}

// LeadSourceResponse masks secret tokens
type LeadSourceResponse struct {
	models.LeadSource
	WebhookURL string `json:"webhook_url"`
}

// ListLeadSources lists connected Pages for an organization
func (a *App) ListLeadSources(r *fastglue.Request) error {
	orgID, _, ok := a.leadAuth(r, models.ActionRead)
	if !ok {
		return nil
	}

	var sources []models.LeadSource
	if err := a.DB.Where("organization_id = ?", orgID).Order("created_at DESC").Find(&sources).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to list lead sources", nil, "")
	}

	webhookURL := a.leadWebhookURL()
	resp := make([]LeadSourceResponse, len(sources))
	for i, s := range sources {
		resp[i] = LeadSourceResponse{LeadSource: s, WebhookURL: webhookURL}
	}
	return r.SendEnvelope(resp)
}

// CreateLeadSource connects a new Facebook Page
func (a *App) CreateLeadSource(r *fastglue.Request) error {
	orgID, userID, ok := a.leadAuth(r, models.ActionWrite)
	if !ok {
		return nil
	}

	var req LeadSourceCreateRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	req.Name = strings.TrimSpace(req.Name)
	req.PageID = strings.TrimSpace(req.PageID)
	req.PageAccessToken = strings.TrimSpace(req.PageAccessToken)
	req.AppSecret = strings.TrimSpace(req.AppSecret)

	if req.Name == "" || req.PageID == "" || req.PageAccessToken == "" || req.AppSecret == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Name, Page ID, Page Access Token and App Secret are required", nil, "")
	}
	apiVersion, valid := normalizeLeadAPIVersion(req.APIVersion)
	if !valid {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "API version must look like v21.0", nil, "")
	}
	defaultCC := strings.TrimSpace(req.DefaultCountryCode)
	if defaultCC == "" {
		defaultCC = "91"
	}

	// A Page may only be connected once across all organizations. The default
	// GORM scope ignores soft-deleted rows, matching the partial unique index.
	var existingCount int64
	if err := a.DB.Model(&models.LeadSource{}).Where("page_id = ?", req.PageID).Count(&existingCount).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to check existing lead sources", nil, "")
	}
	if existingCount > 0 {
		return r.SendErrorEnvelope(fasthttp.StatusConflict, "Page ID is already connected", nil, "")
	}

	ctx, cancel := context.WithTimeout(r.RequestCtx, 10*time.Second)
	defer cancel()
	pageProfile, err := a.WhatsApp.GetPage(ctx, apiVersion, req.PageID, req.PageAccessToken)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, fmt.Sprintf("Failed to validate Page token: %v", err), nil, "")
	}

	verifyBytes := make([]byte, 16)
	if _, err := rand.Read(verifyBytes); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to generate verify token", nil, "")
	}

	source := models.LeadSource{
		BaseModel:          models.BaseModel{ID: uuid.New()},
		OrganizationID:     orgID,
		Name:               req.Name,
		PageID:             req.PageID,
		PageName:           pageProfile.Name,
		AppID:              strings.TrimSpace(req.AppID),
		AppSecret:          req.AppSecret,
		PageAccessToken:    req.PageAccessToken,
		WebhookVerifyToken: hex.EncodeToString(verifyBytes),
		APIVersion:         apiVersion,
		DefaultCountryCode: defaultCC,
		IsActive:           true,
		CreatedByID:        &userID,
	}
	if err := source.EncryptSecrets(a.Config.App.EncryptionKey); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to encrypt credentials", nil, "")
	}
	if err := a.DB.Create(&source).Error; err != nil {
		if isUniqueViolation(err) {
			return r.SendErrorEnvelope(fasthttp.StatusConflict, "Page ID is already connected", nil, "")
		}
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to save lead source", nil, "")
	}

	a.InvalidateLeadSourceCache(source.PageID)
	return r.SendEnvelope(LeadSourceResponse{LeadSource: source, WebhookURL: a.leadWebhookURL()})
}

// GetLeadSource retrieves a single lead source
func (a *App) GetLeadSource(r *fastglue.Request) error {
	orgID, _, ok := a.leadAuth(r, models.ActionRead)
	if !ok {
		return nil
	}
	source := a.loadLeadSource(r, orgID)
	if source == nil {
		return nil
	}
	return r.SendEnvelope(LeadSourceResponse{LeadSource: *source, WebhookURL: a.leadWebhookURL()})
}

// UpdateLeadSource updates an existing lead source
func (a *App) UpdateLeadSource(r *fastglue.Request) error {
	orgID, _, ok := a.leadAuth(r, models.ActionWrite)
	if !ok {
		return nil
	}
	source := a.loadLeadSource(r, orgID)
	if source == nil {
		return nil
	}

	var req LeadSourceUpdateRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	if name := strings.TrimSpace(req.Name); name != "" {
		source.Name = name
	}
	if appID := strings.TrimSpace(req.AppID); appID != "" {
		source.AppID = appID
	}
	if strings.TrimSpace(req.APIVersion) != "" {
		apiVersion, valid := normalizeLeadAPIVersion(req.APIVersion)
		if !valid {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "API version must look like v21.0", nil, "")
		}
		source.APIVersion = apiVersion
	}
	if cc := strings.TrimSpace(req.DefaultCountryCode); cc != "" {
		source.DefaultCountryCode = cc
	}
	if req.IsActive != nil {
		source.IsActive = *req.IsActive
	}

	source.DecryptSecrets(a.Config.App.EncryptionKey)
	if token := strings.TrimSpace(req.PageAccessToken); token != "" {
		source.PageAccessToken = token
	}
	if secret := strings.TrimSpace(req.AppSecret); secret != "" {
		source.AppSecret = secret
	}
	// The App Secret verifies webhook signatures; a source must never be left
	// without one (legacy rows created before it was required must add it).
	if source.AppSecret == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "App Secret is required", nil, "")
	}
	if err := source.EncryptSecrets(a.Config.App.EncryptionKey); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to encrypt credentials", nil, "")
	}

	if err := a.DB.Save(source).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to update lead source", nil, "")
	}

	a.InvalidateLeadSourceCache(source.PageID)
	return r.SendEnvelope(source)
}

// DeleteLeadSource unsubscribes the app from the Page and soft-deletes the
// source together with its form mappings.
func (a *App) DeleteLeadSource(r *fastglue.Request) error {
	orgID, _, ok := a.leadAuth(r, models.ActionDelete)
	if !ok {
		return nil
	}
	source := a.loadLeadSource(r, orgID)
	if source == nil {
		return nil
	}
	source.DecryptSecrets(a.Config.App.EncryptionKey)

	ctx, cancel := context.WithTimeout(r.RequestCtx, 5*time.Second)
	defer cancel()
	if err := a.WhatsApp.UnsubscribePage(ctx, source.APIVersion, source.PageID, source.PageAccessToken); err != nil {
		// Deletion proceeds: the token may already be revoked. Meta keeps the
		// app installed until it is removed in Page settings.
		a.Log.Warn("Failed to unsubscribe Page from leadgen webhooks", "page_id", source.PageID, "error", err)
	}

	err := a.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("lead_source_id = ? AND organization_id = ?", source.ID, orgID).Delete(&models.LeadFormMapping{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.LeadSource{}, "id = ? AND organization_id = ?", source.ID, orgID).Error
	})
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to delete lead source", nil, "")
	}

	a.InvalidateLeadSourceCache(source.PageID)
	return r.SendEnvelope(map[string]bool{"success": true})
}
