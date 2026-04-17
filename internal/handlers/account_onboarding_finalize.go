package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/pkg/whatsapp"
)

func (a *App) buildOnboardingReadiness(session *models.WhatsAppOnboardingSession, connection *accountConnectionCheck, phoneStatus *whatsapp.PhoneNumberStatus) OnboardingReadinessStatus {
	readiness := OnboardingReadinessStatus{
		Status:  onboardingStatusReady,
		Summary: "Onboarding is complete and the account is ready to use.",
	}

	preflight := a.getOnboardingStepState(session, onboardingStepPreflight)
	phoneSetup := a.getOnboardingStepState(session, onboardingStepPhoneSetup)
	webhooks := a.getOnboardingStepState(session, onboardingStepWebhooks)
	if preflight.Status != onboardingStateCompleted {
		readiness.Status = onboardingStatusActionRequired
		readiness.Summary = "Platform prerequisites are incomplete."
	}
	if readiness.Status == onboardingStatusReady && phoneSetup.Status != onboardingStateCompleted {
		readiness.Status = onboardingStatusActionRequired
		readiness.Summary = phoneSetup.Summary
	}

	validationStatus, _ := webhooks.Details["validation_status"].(string)
	subscriptionStatus, _ := webhooks.Details["subscription_status"].(string)
	if readiness.Status == onboardingStatusReady && (validationStatus != onboardingStateCompleted || subscriptionStatus != onboardingStateCompleted) {
		readiness.Status = onboardingStatusActionRequired
		readiness.Summary = "Webhook validation and subscription must both succeed before the account is ready."
	}
	if readiness.Status == onboardingStatusReady && (connection == nil || !connection.Success) {
		connectionError := ""
		if connection != nil {
			connectionError = connection.Error
		}
		readiness.Status = onboardingStatusActionRequired
		readiness.Summary = firstNonEmpty(connectionError, "Connection test failed.")
	}

	checkpoints := make([]OnboardingCheckpointStatus, 0, 3)
	if phoneStatus != nil && strings.TrimSpace(phoneStatus.VerifiedName) == "" {
		checkpoints = append(checkpoints, OnboardingCheckpointStatus{
			Key:     "display_name_review",
			Label:   "Display name review",
			Status:  onboardingStateWaitingOnMeta,
			Summary: "Meta has not returned a verified display name yet.",
		})
	}

	metadata := session.Metadata
	if metadata == nil {
		metadata = models.JSONB{}
	}
	if status, ok := metadata["business_verification_status"].(string); ok && status == onboardingStateWaitingOnMeta {
		checkpoints = append(checkpoints, OnboardingCheckpointStatus{
			Key:     "business_verification",
			Label:   "Business verification",
			Status:  onboardingStateWaitingOnMeta,
			Summary: "Meta business verification is still pending.",
		})
	} else {
		checkpoints = append(checkpoints, OnboardingCheckpointStatus{
			Key:     "business_verification",
			Label:   "Business verification",
			Status:  "unknown",
			Summary: "Meta does not reliably expose this checkpoint in the Cloud API endpoints used here. Review Business Manager before production launch.",
		})
	}

	if connection != nil && connection.IsTestNumber {
		checkpoints = append(checkpoints, OnboardingCheckpointStatus{
			Key:     "number_type",
			Label:   "Production number",
			Status:  onboardingStateActionRequired,
			Summary: "This is a sandbox number and cannot serve as the final production sender.",
		})
	}

	for _, checkpoint := range checkpoints {
		if checkpoint.Status == onboardingStateWaitingOnMeta && readiness.Status == onboardingStatusReady {
			readiness.Status = onboardingStatusWaitingOnMeta
			readiness.Summary = "Internal setup is complete, but Meta-controlled checkpoints are still pending."
		}
		if checkpoint.Status == onboardingStateActionRequired {
			readiness.Status = onboardingStatusActionRequired
			readiness.Summary = checkpoint.Summary
		}
	}
	readiness.ExternalCheckpoints = checkpoints
	return readiness
}

func (a *App) runAccountConnectionCheck(account *models.WhatsAppAccount) (*accountConnectionCheck, error) {
	if err := a.validateAccountCredentials(account.PhoneID, account.BusinessID, account.AccessToken, account.APIVersion); err != nil {
		return &accountConnectionCheck{
			Success: false,
			Error:   "Account credential validation failed. Check your access token and phone ID.",
		}, nil
	}

	requestURL := fmt.Sprintf("%s/%s/%s?fields=display_phone_number,verified_name,code_verification_status,account_mode,quality_rating,messaging_limit_tier",
		a.Config.WhatsApp.BaseURL, account.APIVersion, account.PhoneID)
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+account.AccessToken)

	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return &accountConnectionCheck{
			Success: false,
			Error:   "Failed to connect to WhatsApp API",
		}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return &accountConnectionCheck{
			Success: false,
			Error:   "API error",
		}, nil
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	accountMode, _ := result["account_mode"].(string)
	verificationStatus, _ := result["code_verification_status"].(string)
	verifiedName, _ := result["verified_name"].(string)
	isTestNumber := accountMode == "SANDBOX" || strings.EqualFold(verifiedName, "Test Number")

	check := &accountConnectionCheck{
		Success:                true,
		DisplayPhoneNumber:     toString(result["display_phone_number"]),
		VerifiedName:           verifiedName,
		QualityRating:          toString(result["quality_rating"]),
		MessagingLimitTier:     toString(result["messaging_limit_tier"]),
		CodeVerificationStatus: verificationStatus,
		AccountMode:            accountMode,
		IsTestNumber:           isTestNumber,
	}
	if isTestNumber {
		check.Warning = "This is a test/sandbox number. Not suitable for production use."
	} else if verificationStatus == "EXPIRED" {
		check.Warning = "Phone verification has expired. Consider re-verifying in Meta before production use."
	}
	return check, nil
}
