package handlers

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/crypto"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

const (
	metaOnboardingConfigKey     = "meta_onboarding_config"
	defaultMetaGraphAPIVersion  = "v21.0"
)

// MetaOnboardingConfig is the super-admin-managed platform configuration used by the onboarding wizard.
type MetaOnboardingConfig struct {
	MetaAppID              string   `json:"meta_app_id"`
	EmbeddedSignupConfigID string   `json:"embedded_signup_config_id"`
	GraphAPIVersion        string   `json:"graph_api_version"`
	PublicWebhookBaseURL   string   `json:"public_webhook_base_url"`
	RequiredScopes         []string `json:"required_scopes"`
	HasAppSecret           bool     `json:"has_app_secret"`
	IsConfigured           bool     `json:"is_configured"`
	CallbackURL            string   `json:"callback_url"`
}

type metaOnboardingConfigRequest struct {
	MetaAppID              string   `json:"meta_app_id"`
	MetaAppSecret          string   `json:"meta_app_secret"`
	EmbeddedSignupConfigID string   `json:"embedded_signup_config_id"`
	GraphAPIVersion        string   `json:"graph_api_version"`
	PublicWebhookBaseURL   string   `json:"public_webhook_base_url"`
	RequiredScopes         []string `json:"required_scopes"`
}

type storedMetaOnboardingConfig struct {
	MetaAppID              string   `json:"meta_app_id"`
	MetaAppSecret          string   `json:"meta_app_secret"`
	EmbeddedSignupConfigID string   `json:"embedded_signup_config_id"`
	GraphAPIVersion        string   `json:"graph_api_version"`
	PublicWebhookBaseURL   string   `json:"public_webhook_base_url"`
	RequiredScopes         []string `json:"required_scopes"`
}

// GetMetaOnboardingConfig returns the app-level Meta onboarding configuration.
func (a *App) GetMetaOnboardingConfig(r *fastglue.Request) error {
	userID, err := a.requireSuperAdmin(r)
	if err != nil {
		return nil
	}

	cfg, err := a.getMetaOnboardingConfig()
	if err != nil {
		a.Log.Error("Failed to load Meta onboarding config", "error", err, "user_id", userID)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to load Meta onboarding configuration", nil, "")
	}

	return r.SendEnvelope(map[string]any{
		"config": cfg,
	})
}

// UpdateMetaOnboardingConfig updates the app-level Meta onboarding configuration.
func (a *App) UpdateMetaOnboardingConfig(r *fastglue.Request) error {
	userID, err := a.requireSuperAdmin(r)
	if err != nil {
		return nil
	}

	var req metaOnboardingConfigRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	existing, err := a.getStoredMetaOnboardingConfig()
	if err != nil {
		a.Log.Error("Failed to read existing Meta onboarding config", "error", err, "user_id", userID)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to update Meta onboarding configuration", nil, "")
	}

	merged := storedMetaOnboardingConfig{
		MetaAppID:              strings.TrimSpace(req.MetaAppID),
		MetaAppSecret:          strings.TrimSpace(req.MetaAppSecret),
		EmbeddedSignupConfigID: strings.TrimSpace(req.EmbeddedSignupConfigID),
		GraphAPIVersion:        strings.TrimSpace(req.GraphAPIVersion),
		PublicWebhookBaseURL:   strings.TrimSpace(req.PublicWebhookBaseURL),
		RequiredScopes:         req.RequiredScopes,
	}
	if merged.MetaAppSecret == "" {
		merged.MetaAppSecret = existing.MetaAppSecret
	}
	merged = normalizeStoredMetaOnboardingConfig(merged)

	if merged.MetaAppID == "" || merged.EmbeddedSignupConfigID == "" || merged.PublicWebhookBaseURL == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "meta_app_id, embedded_signup_config_id, and public_webhook_base_url are required", nil, "")
	}
	if merged.MetaAppSecret == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "meta_app_secret is required", nil, "")
	}
	if !strings.HasPrefix(strings.ToLower(metaOnboardingCallbackURL(merged.PublicWebhookBaseURL)), "https://") {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "The public webhook URL must use HTTPS", nil, "")
	}

	if err := a.saveStoredMetaOnboardingConfig(merged); err != nil {
		a.Log.Error("Failed to save Meta onboarding config", "error", err, "user_id", userID)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to update Meta onboarding configuration", nil, "")
	}

	cfg, err := a.getMetaOnboardingConfig()
	if err != nil {
		a.Log.Error("Failed to load saved Meta onboarding config", "error", err, "user_id", userID)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to load Meta onboarding configuration", nil, "")
	}

	return r.SendEnvelope(map[string]any{
		"config":   cfg,
		"message":  "Meta onboarding configuration updated successfully",
	})
}

func (a *App) requireSuperAdmin(r *fastglue.Request) (uuid.UUID, error) {
	userID, ok := r.RequestCtx.UserValue("user_id").(uuid.UUID)
	if !ok {
		_ = r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
		return uuid.Nil, errEnvelopeSent
	}
	if !a.IsSuperAdmin(userID) {
		_ = r.SendErrorEnvelope(fasthttp.StatusForbidden, "Super admin access required", nil, "")
		return uuid.Nil, errEnvelopeSent
	}
	return userID, nil
}

func (a *App) getMetaOnboardingConfig() (*MetaOnboardingConfig, error) {
	stored, err := a.getStoredMetaOnboardingConfig()
	if err != nil {
		return nil, err
	}

	cfg := &MetaOnboardingConfig{
		MetaAppID:              stored.MetaAppID,
		EmbeddedSignupConfigID: stored.EmbeddedSignupConfigID,
		GraphAPIVersion:        stored.GraphAPIVersion,
		PublicWebhookBaseURL:   stored.PublicWebhookBaseURL,
		RequiredScopes:         stored.RequiredScopes,
		HasAppSecret:           strings.TrimSpace(stored.MetaAppSecret) != "",
	}
	cfg.CallbackURL = metaOnboardingCallbackURL(cfg.PublicWebhookBaseURL)
	cfg.IsConfigured = cfg.MetaAppID != "" && cfg.EmbeddedSignupConfigID != "" && cfg.PublicWebhookBaseURL != "" && cfg.HasAppSecret
	return cfg, nil
}

func (a *App) getStoredMetaOnboardingConfig() (*storedMetaOnboardingConfig, error) {
	var setting models.SystemSetting
	if err := a.DB.Where("key = ?", metaOnboardingConfigKey).First(&setting).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			cfg := normalizeStoredMetaOnboardingConfig(storedMetaOnboardingConfig{})
			return &cfg, nil
		}
		return nil, err
	}

	cfg := normalizeStoredMetaOnboardingConfig(storedMetaOnboardingConfig{})
	if len(setting.Value) > 0 {
		if err := decodeJSONB(setting.Value, &cfg); err != nil {
			return nil, err
		}
	}
	cfg = normalizeStoredMetaOnboardingConfig(cfg)
	crypto.DecryptFields(a.Config.App.EncryptionKey, &cfg.MetaAppSecret)
	return &cfg, nil
}

func (a *App) saveStoredMetaOnboardingConfig(cfg storedMetaOnboardingConfig) error {
	cfg = normalizeStoredMetaOnboardingConfig(cfg)

	encSecret, err := crypto.Encrypt(cfg.MetaAppSecret, a.Config.App.EncryptionKey)
	if err != nil {
		return err
	}
	cfg.MetaAppSecret = encSecret

	value, err := structToJSONB(cfg)
	if err != nil {
		return err
	}

	var setting models.SystemSetting
	if err := a.DB.Where("key = ?", metaOnboardingConfigKey).First(&setting).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		setting = models.SystemSetting{
			Key:   metaOnboardingConfigKey,
			Value: value,
		}
		return a.DB.Create(&setting).Error
	}

	setting.Value = value
	return a.DB.Save(&setting).Error
}

func normalizeStoredMetaOnboardingConfig(cfg storedMetaOnboardingConfig) storedMetaOnboardingConfig {
	cfg.MetaAppID = strings.TrimSpace(cfg.MetaAppID)
	cfg.MetaAppSecret = strings.TrimSpace(cfg.MetaAppSecret)
	cfg.EmbeddedSignupConfigID = strings.TrimSpace(cfg.EmbeddedSignupConfigID)
	cfg.GraphAPIVersion = strings.TrimSpace(cfg.GraphAPIVersion)
	if cfg.GraphAPIVersion == "" {
		cfg.GraphAPIVersion = defaultMetaGraphAPIVersion
	}
	cfg.PublicWebhookBaseURL = normalizePublicWebhookBaseURL(cfg.PublicWebhookBaseURL)
	cfg.RequiredScopes = normalizeMetaScopes(cfg.RequiredScopes)
	return cfg
}

func normalizePublicWebhookBaseURL(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, "/")
	value = strings.TrimSuffix(value, "/api/webhook")
	return value
}

func metaOnboardingCallbackURL(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return ""
	}
	if strings.HasSuffix(baseURL, "/api/webhook") {
		return baseURL
	}
	return strings.TrimSuffix(baseURL, "/") + "/api/webhook"
}

func normalizeMetaScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return []string{
			"whatsapp_business_management",
			"whatsapp_business_messaging",
			"business_management",
		}
	}

	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		normalized = append(normalized, scope)
	}
	if len(normalized) == 0 {
		return normalizeMetaScopes(nil)
	}
	return normalized
}

func structToJSONB(v any) (models.JSONB, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var result models.JSONB
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func decodeJSONB(raw models.JSONB, target any) error {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, target)
}
