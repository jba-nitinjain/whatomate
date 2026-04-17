package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
)

const (
	onboardingModeEmbedded = "embedded_signup"
	onboardingModeManual   = "manual_import"

	onboardingStepPreflight        = "preflight"
	onboardingStepAssetAcquisition = "asset_acquisition"
	onboardingStepAssetImport      = "asset_import"
	onboardingStepPhoneSetup       = "phone_setup"
	onboardingStepWebhooks         = "webhooks"
	onboardingStepConnectionTest   = "connection_test"
	onboardingStepFinalStatus      = "final_status"

	onboardingStatePending        = "pending"
	onboardingStateInProgress     = "in_progress"
	onboardingStateCompleted      = "completed"
	onboardingStateActionRequired = "action_required"
	onboardingStateWaitingOnMeta  = "waiting_on_meta"

	onboardingStatusInProgress     = "in_progress"
	onboardingStatusReady          = "ready"
	onboardingStatusActionRequired = "action_required"
	onboardingStatusWaitingOnMeta  = "waiting_on_meta"
	onboardingStatusFailed         = "failed"
)

// OnboardingStepState tracks the persisted state of a single onboarding step.
type OnboardingStepState struct {
	Status    string         `json:"status"`
	Summary   string         `json:"summary,omitempty"`
	UpdatedAt string         `json:"updated_at,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

// OnboardingCheckpointStatus models Meta-owned readiness states that Whatomate can track but not complete.
type OnboardingCheckpointStatus struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Status  string `json:"status"`
	Summary string `json:"summary,omitempty"`
}

// OnboardingReadinessStatus is the final readiness summary for a wizard session.
type OnboardingReadinessStatus struct {
	Status              string                       `json:"status"`
	Summary             string                       `json:"summary,omitempty"`
	ExternalCheckpoints []OnboardingCheckpointStatus `json:"external_checkpoints,omitempty"`
}

// WhatsAppOnboardingSessionResponse is the API response shape returned by all onboarding session endpoints.
type WhatsAppOnboardingSessionResponse struct {
	ID                 uuid.UUID                      `json:"id"`
	OrganizationID     uuid.UUID                      `json:"organization_id"`
	AccountID          *uuid.UUID                     `json:"account_id,omitempty"`
	Mode               string                         `json:"mode"`
	Status             string                         `json:"status"`
	CurrentStep        string                         `json:"current_step"`
	AccountName        string                         `json:"account_name"`
	AppID              string                         `json:"app_id"`
	PhoneID            string                         `json:"phone_id"`
	BusinessID         string                         `json:"business_id"`
	WebhookVerifyToken string                         `json:"webhook_verify_token"`
	APIVersion         string                         `json:"api_version"`
	HasAccessToken     bool                           `json:"has_access_token"`
	HasAppSecret       bool                           `json:"has_app_secret"`
	StepState          map[string]OnboardingStepState `json:"step_state"`
	Readiness          OnboardingReadinessStatus      `json:"readiness"`
	Metadata           map[string]any                 `json:"metadata,omitempty"`
	LastError          string                         `json:"last_error,omitempty"`
	CompletedAt        string                         `json:"completed_at,omitempty"`
	CreatedAt          string                         `json:"created_at"`
	UpdatedAt          string                         `json:"updated_at"`
}

type createOnboardingSessionRequest struct {
	Mode        string `json:"mode"`
	AccountName string `json:"account_name"`
}

// EmbeddedSignupCompletionPayload accepts either an exchangeable Meta code or a direct access token.
type EmbeddedSignupCompletionPayload struct {
	AccountName        string         `json:"account_name"`
	Code               string         `json:"code"`
	AccessToken        string         `json:"access_token"`
	AppID              string         `json:"app_id"`
	PhoneID            string         `json:"phone_id"`
	BusinessID         string         `json:"business_id"`
	WebhookVerifyToken string         `json:"webhook_verify_token"`
	APIVersion         string         `json:"api_version"`
	Metadata           map[string]any `json:"metadata"`
}

type manualImportPayload struct {
	AccountName        string `json:"account_name"`
	AppID              string `json:"app_id"`
	AppSecret          string `json:"app_secret"`
	PhoneID            string `json:"phone_id"`
	BusinessID         string `json:"business_id"`
	AccessToken        string `json:"access_token"`
	WebhookVerifyToken string `json:"webhook_verify_token"`
	APIVersion         string `json:"api_version"`
}

type onboardingRequestCodeRequest struct {
	CodeMethod string `json:"code_method"`
	Language   string `json:"language"`
}

type onboardingVerifyCodeRequest struct {
	Code string `json:"code"`
}

type onboardingRegisterPhoneRequest struct {
	Pin                       string `json:"pin"`
	BackupPassword            string `json:"backup_password"`
	BackupData                string `json:"backup_data"`
	DataLocalizationRegion    string `json:"data_localization_region"`
	MetaStoreRetentionMinutes *int   `json:"meta_store_retention_minutes"`
}

type onboardingAccountImportInput struct {
	AccountName        string
	AppID              string
	AppSecret          string
	PhoneID            string
	BusinessID         string
	AccessToken        string
	WebhookVerifyToken string
	APIVersion         string
}

type accountConnectionCheck struct {
	Success                bool   `json:"success"`
	Error                  string `json:"error,omitempty"`
	DisplayPhoneNumber     string `json:"display_phone_number,omitempty"`
	VerifiedName           string `json:"verified_name,omitempty"`
	QualityRating          string `json:"quality_rating,omitempty"`
	MessagingLimitTier     string `json:"messaging_limit_tier,omitempty"`
	CodeVerificationStatus string `json:"code_verification_status,omitempty"`
	AccountMode            string `json:"account_mode,omitempty"`
	IsTestNumber           bool   `json:"is_test_number,omitempty"`
	Warning                string `json:"warning,omitempty"`
}

func defaultOnboardingStepStates() map[string]OnboardingStepState {
	return map[string]OnboardingStepState{
		onboardingStepPreflight:        {Status: onboardingStatePending},
		onboardingStepAssetAcquisition: {Status: onboardingStatePending},
		onboardingStepAssetImport:      {Status: onboardingStatePending},
		onboardingStepPhoneSetup:       {Status: onboardingStatePending},
		onboardingStepWebhooks:         {Status: onboardingStatePending},
		onboardingStepConnectionTest:   {Status: onboardingStatePending},
		onboardingStepFinalStatus:      {Status: onboardingStatePending},
	}
}

func normalizeOnboardingMode(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case onboardingModeManual:
		return onboardingModeManual
	default:
		return onboardingModeEmbedded
	}
}

func onboardingAPIVersion(value string, cfg *MetaOnboardingConfig) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if cfg != nil && strings.TrimSpace(cfg.GraphAPIVersion) != "" {
		return strings.TrimSpace(cfg.GraphAPIVersion)
	}
	return defaultMetaGraphAPIVersion
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func mergeJSONB(base models.JSONB, extra map[string]any) models.JSONB {
	if base == nil {
		base = models.JSONB{}
	}
	for key, value := range extra {
		base[key] = value
	}
	return base
}

func mergeMaps(base map[string]any, extra map[string]any) map[string]any {
	if base == nil {
		base = map[string]any{}
	}
	for key, value := range extra {
		base[key] = value
	}
	return base
}

func isAlreadySubscribedError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "already") && strings.Contains(message, "subscrib")
}

func truncateAccountName(name string) string {
	name = strings.TrimSpace(name)
	if len(name) <= 100 {
		return name
	}
	return strings.TrimSpace(name[:100])
}

func trimForName(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 8 {
		return value[len(value)-8:]
	}
	return value
}

func formatFallbackAccountName(phoneID string) string {
	phoneID = trimForName(phoneID)
	if phoneID == "" {
		return "WhatsApp account"
	}
	return fmt.Sprintf("WhatsApp %s", phoneID)
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
