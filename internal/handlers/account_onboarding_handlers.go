package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/pkg/whatsapp"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// CreateWhatsAppOnboardingSession creates a resumable onboarding session for the current organization.
func (a *App) CreateWhatsAppOnboardingSession(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	var req createOnboardingSessionRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	metaCfg, err := a.getMetaOnboardingConfig()
	if err != nil {
		a.Log.Error("Failed to load Meta onboarding config", "error", err, "org_id", orgID)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to create onboarding session", nil, "")
	}

	session := models.WhatsAppOnboardingSession{
		OrganizationID: orgID,
		Mode:           normalizeOnboardingMode(req.Mode),
		Status:         onboardingStatusInProgress,
		CurrentStep:    onboardingStepPreflight,
		AccountName:    strings.TrimSpace(req.AccountName),
		APIVersion:     onboardingAPIVersion("", metaCfg),
		StepState:      models.JSONB{},
		Readiness:      models.JSONB{},
		Metadata:       models.JSONB{},
	}
	a.applyOnboardingPreflight(&session, metaCfg)

	if err := a.DB.Create(&session).Error; err != nil {
		a.Log.Error("Failed to create onboarding session", "error", err, "org_id", orgID)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to create onboarding session", nil, "")
	}

	return r.SendEnvelope(map[string]any{
		"session": a.onboardingSessionToResponse(&session),
	})
}

// GetWhatsAppOnboardingSession returns a persisted onboarding session.
func (a *App) GetWhatsAppOnboardingSession(r *fastglue.Request) error {
	session, err := a.resolveOnboardingSessionByID(r)
	if err != nil {
		return nil
	}

	metaCfg, cfgErr := a.getMetaOnboardingConfig()
	if cfgErr == nil {
		a.applyOnboardingPreflight(session, metaCfg)
	}

	return r.SendEnvelope(map[string]any{
		"session": a.onboardingSessionToResponse(session),
	})
}

// CompleteEmbeddedSignupOnboarding stores the assets returned by Meta embedded signup and imports the account.
func (a *App) CompleteEmbeddedSignupOnboarding(r *fastglue.Request) error {
	session, err := a.resolveOnboardingSessionByID(r)
	if err != nil {
		return nil
	}

	var req EmbeddedSignupCompletionPayload
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	metaCfg, err := a.getStoredMetaOnboardingConfig()
	if err != nil {
		a.Log.Error("Failed to load stored Meta onboarding config", "error", err, "session_id", session.ID)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to complete embedded signup", nil, "")
	}

	accessToken := strings.TrimSpace(req.AccessToken)
	if accessToken == "" {
		code := strings.TrimSpace(req.Code)
		if code == "" {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Either code or access_token is required", nil, "")
		}
		if metaCfg.MetaAppID == "" || metaCfg.MetaAppSecret == "" {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "App-level Meta configuration is incomplete. Set meta_app_id and meta_app_secret first.", nil, "")
		}

		tokenResp, err := a.WhatsApp.ExchangeEmbeddedSignupCode(context.Background(), metaCfg.MetaAppID, metaCfg.MetaAppSecret, code)
		if err != nil {
			a.setOnboardingFailure(session, onboardingStepAssetAcquisition, err)
			_ = a.DB.Save(session).Error
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
		}
		accessToken = strings.TrimSpace(tokenResp.AccessToken)
	}

	input := onboardingAccountImportInput{
		AccountName:        req.AccountName,
		AppID:              firstNonEmpty(req.AppID, metaCfg.MetaAppID),
		AppSecret:          metaCfg.MetaAppSecret,
		PhoneID:            strings.TrimSpace(req.PhoneID),
		BusinessID:         strings.TrimSpace(req.BusinessID),
		AccessToken:        accessToken,
		WebhookVerifyToken: strings.TrimSpace(req.WebhookVerifyToken),
		APIVersion:         onboardingAPIVersion(req.APIVersion, nil),
	}
	if input.APIVersion == "" {
		input.APIVersion = onboardingAPIVersion(metaCfg.GraphAPIVersion, nil)
	}

	if err := a.importOnboardingAssets(session, input, req.Metadata, onboardingModeEmbedded); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}

	return r.SendEnvelope(map[string]any{
		"session": a.onboardingSessionToResponse(session),
	})
}

// ManualImportWhatsAppOnboarding imports manually provided Meta asset values into an onboarding session.
func (a *App) ManualImportWhatsAppOnboarding(r *fastglue.Request) error {
	session, err := a.resolveOnboardingSessionByID(r)
	if err != nil {
		return nil
	}

	var req manualImportPayload
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	metaCfg, err := a.getStoredMetaOnboardingConfig()
	if err != nil {
		a.Log.Error("Failed to load stored Meta onboarding config", "error", err, "session_id", session.ID)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to import Meta assets", nil, "")
	}

	input := onboardingAccountImportInput{
		AccountName:        req.AccountName,
		AppID:              firstNonEmpty(req.AppID, metaCfg.MetaAppID),
		AppSecret:          firstNonEmpty(req.AppSecret, metaCfg.MetaAppSecret),
		PhoneID:            strings.TrimSpace(req.PhoneID),
		BusinessID:         strings.TrimSpace(req.BusinessID),
		AccessToken:        strings.TrimSpace(req.AccessToken),
		WebhookVerifyToken: strings.TrimSpace(req.WebhookVerifyToken),
		APIVersion:         onboardingAPIVersion(req.APIVersion, nil),
	}
	if input.APIVersion == "" {
		input.APIVersion = onboardingAPIVersion(metaCfg.GraphAPIVersion, nil)
	}

	if err := a.importOnboardingAssets(session, input, nil, onboardingModeManual); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}

	return r.SendEnvelope(map[string]any{
		"session": a.onboardingSessionToResponse(session),
	})
}

// RequestOnboardingPhoneCode requests the Meta verification code for the onboarding session's phone number.
func (a *App) RequestOnboardingPhoneCode(r *fastglue.Request) error {
	session, err := a.resolveOnboardingSessionByID(r)
	if err != nil {
		return nil
	}

	var req onboardingRequestCodeRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	waAccount, _, err := a.resolveOnboardingWhatsAppAccount(session)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}

	codeMethod := normalizePhoneVerificationMethod(req.CodeMethod)
	if codeMethod == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "code_method must be SMS or VOICE", nil, "")
	}
	language := strings.TrimSpace(req.Language)
	if language == "" {
		language = "en_US"
	}

	if err := a.WhatsApp.RequestPhoneNumberVerificationCode(context.Background(), waAccount, codeMethod, language); err != nil {
		a.setOnboardingFailure(session, onboardingStepPhoneSetup, err)
		_ = a.DB.Save(session).Error
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}

	step := a.getOnboardingStepState(session, onboardingStepPhoneSetup)
	step.Status = onboardingStateInProgress
	step.Summary = "Verification code requested. Confirm the code from Meta to continue."
	step.UpdatedAt = nowRFC3339()
	step.Details = mergeMaps(step.Details, map[string]any{
		"code_request_status":   onboardingStateCompleted,
		"code_request_method":   codeMethod,
		"code_request_language": language,
	})
	a.setOnboardingStepState(session, onboardingStepPhoneSetup, step)
	session.CurrentStep = onboardingStepPhoneSetup
	session.Status = onboardingStatusInProgress
	session.LastError = ""
	if err := a.DB.Save(session).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to save onboarding progress", nil, "")
	}

	return r.SendEnvelope(map[string]any{
		"session": a.onboardingSessionToResponse(session),
	})
}

// VerifyOnboardingPhoneCode confirms the OTP from Meta for the session phone number.
func (a *App) VerifyOnboardingPhoneCode(r *fastglue.Request) error {
	session, err := a.resolveOnboardingSessionByID(r)
	if err != nil {
		return nil
	}

	var req onboardingVerifyCodeRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	waAccount, _, err := a.resolveOnboardingWhatsAppAccount(session)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}

	code := strings.TrimSpace(req.Code)
	if code == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Verification code is required", nil, "")
	}

	result, err := a.WhatsApp.VerifyPhoneNumberVerificationCode(context.Background(), waAccount, code)
	if err != nil {
		a.setOnboardingFailure(session, onboardingStepPhoneSetup, err)
		_ = a.DB.Save(session).Error
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}

	step := a.getOnboardingStepState(session, onboardingStepPhoneSetup)
	step.Status = onboardingStateInProgress
	step.Summary = "Verification code confirmed. Register the phone number to enable messaging."
	step.UpdatedAt = nowRFC3339()
	step.Details = mergeMaps(step.Details, map[string]any{
		"verification_status": onboardingStateCompleted,
		"verification_id":     result.ID,
	})
	a.setOnboardingStepState(session, onboardingStepPhoneSetup, step)
	session.CurrentStep = onboardingStepPhoneSetup
	session.Status = onboardingStatusInProgress
	session.LastError = ""
	if err := a.DB.Save(session).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to save onboarding progress", nil, "")
	}

	return r.SendEnvelope(map[string]any{
		"session": a.onboardingSessionToResponse(session),
	})
}

// RegisterOnboardingPhone completes Meta phone registration for the session.
func (a *App) RegisterOnboardingPhone(r *fastglue.Request) error {
	session, err := a.resolveOnboardingSessionByID(r)
	if err != nil {
		return nil
	}

	var req onboardingRegisterPhoneRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	waAccount, _, err := a.resolveOnboardingWhatsAppAccount(session)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}

	if !isSixDigitPIN(req.Pin) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "PIN must be exactly 6 digits", nil, "")
	}
	hasBackupPassword := strings.TrimSpace(req.BackupPassword) != ""
	hasBackupData := strings.TrimSpace(req.BackupData) != ""
	if hasBackupPassword != hasBackupData {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "backup_password and backup_data must be provided together", nil, "")
	}

	input := whatsapp.RegisterPhoneNumberRequest{
		Pin:                       strings.TrimSpace(req.Pin),
		DataLocalizationRegion:    strings.TrimSpace(req.DataLocalizationRegion),
		MetaStoreRetentionMinutes: req.MetaStoreRetentionMinutes,
	}
	if hasBackupPassword && hasBackupData {
		input.Backup = &whatsapp.BusinessAccountBackup{
			Password: strings.TrimSpace(req.BackupPassword),
			Data:     strings.TrimSpace(req.BackupData),
		}
	}

	if err := a.WhatsApp.RegisterPhoneNumber(context.Background(), waAccount, input); err != nil {
		a.setOnboardingFailure(session, onboardingStepPhoneSetup, err)
		_ = a.DB.Save(session).Error
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}

	phoneStatus, _ := a.WhatsApp.GetPhoneNumberStatus(context.Background(), waAccount)
	a.applyPhoneStatusToSession(session, phoneStatus)

	step := a.getOnboardingStepState(session, onboardingStepPhoneSetup)
	step.Status = onboardingStateCompleted
	step.Summary = "Phone number verified and registered with Meta."
	step.UpdatedAt = nowRFC3339()
	step.Details = mergeMaps(step.Details, map[string]any{
		"registration_status": onboardingStateCompleted,
		"two_step_pin_set":    true,
	})
	a.setOnboardingStepState(session, onboardingStepPhoneSetup, step)
	session.CurrentStep = onboardingStepWebhooks
	session.Status = onboardingStatusInProgress
	session.LastError = ""
	if err := a.DB.Save(session).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to save onboarding progress", nil, "")
	}

	return r.SendEnvelope(map[string]any{
		"session": a.onboardingSessionToResponse(session),
	})
}

// ValidateOnboardingWebhook checks that the public callback URL is reachable and that Meta verification succeeds.
func (a *App) ValidateOnboardingWebhook(r *fastglue.Request) error {
	session, err := a.resolveOnboardingSessionByID(r)
	if err != nil {
		return nil
	}

	account, err := a.getOnboardingAccountModel(session)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}
	metaCfg, err := a.getMetaOnboardingConfig()
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to load Meta onboarding configuration", nil, "")
	}
	callbackURL := metaCfg.CallbackURL
	if callbackURL == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "The public webhook callback URL is not configured", nil, "")
	}
	if !strings.HasPrefix(strings.ToLower(callbackURL), "https://") {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "The public webhook callback URL must use HTTPS", nil, "")
	}

	checkURL := callbackURL + "?hub.mode=subscribe&hub.challenge=whatomate-check&hub.verify_token=" + neturl.QueryEscape(account.WebhookVerifyToken)
	req, err := http.NewRequest(http.MethodGet, checkURL, nil)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to build webhook validation request", nil, "")
	}

	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		a.setOnboardingFailure(session, onboardingStepWebhooks, err)
		_ = a.DB.Save(session).Error
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Failed to reach the public webhook callback URL", nil, "")
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != "whatomate-check" {
		err := fmt.Errorf("webhook validation returned status %d", resp.StatusCode)
		a.setOnboardingFailure(session, onboardingStepWebhooks, err)
		step := a.getOnboardingStepState(session, onboardingStepWebhooks)
		step.Status = onboardingStateActionRequired
		step.Summary = "Webhook callback is not reachable or did not validate successfully."
		step.UpdatedAt = nowRFC3339()
		step.Details = mergeMaps(step.Details, map[string]any{
			"callback_url":      callbackURL,
			"validation_status": onboardingStateActionRequired,
			"http_status":       resp.StatusCode,
			"response_body":     strings.TrimSpace(string(body)),
		})
		a.setOnboardingStepState(session, onboardingStepWebhooks, step)
		_ = a.DB.Save(session).Error
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Webhook callback validation failed. Verify the public URL and verify token.", nil, "")
	}

	step := a.getOnboardingStepState(session, onboardingStepWebhooks)
	step.Status = onboardingStateInProgress
	step.Summary = "Webhook callback validated. Subscribe the app to complete webhook setup."
	step.UpdatedAt = nowRFC3339()
	step.Details = mergeMaps(step.Details, map[string]any{
		"callback_url":      callbackURL,
		"validation_status": onboardingStateCompleted,
	})
	a.setOnboardingStepState(session, onboardingStepWebhooks, step)
	session.CurrentStep = onboardingStepWebhooks
	session.Status = onboardingStatusInProgress
	session.LastError = ""
	if err := a.DB.Save(session).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to save onboarding progress", nil, "")
	}

	return r.SendEnvelope(map[string]any{
		"session": a.onboardingSessionToResponse(session),
	})
}

// SubscribeOnboardingWebhooks subscribes the current app to WABA webhooks for the onboarding session.
func (a *App) SubscribeOnboardingWebhooks(r *fastglue.Request) error {
	session, err := a.resolveOnboardingSessionByID(r)
	if err != nil {
		return nil
	}

	waAccount, _, err := a.resolveOnboardingWhatsAppAccount(session)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}

	err = a.WhatsApp.SubscribeApp(context.Background(), waAccount)
	if err != nil && !isAlreadySubscribedError(err) {
		a.setOnboardingFailure(session, onboardingStepWebhooks, err)
		_ = a.DB.Save(session).Error
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}

	step := a.getOnboardingStepState(session, onboardingStepWebhooks)
	stepStatus := onboardingStateCompleted
	if validationStatus, _ := step.Details["validation_status"].(string); validationStatus != onboardingStateCompleted {
		stepStatus = onboardingStateInProgress
	}
	step.Status = stepStatus
	step.Summary = "Webhook subscription completed."
	if stepStatus != onboardingStateCompleted {
		step.Summary = "Webhook subscription completed. Validate the public callback URL before finalizing."
	}
	step.UpdatedAt = nowRFC3339()
	step.Details = mergeMaps(step.Details, map[string]any{
		"subscription_status": onboardingStateCompleted,
	})
	a.setOnboardingStepState(session, onboardingStepWebhooks, step)
	session.CurrentStep = onboardingStepConnectionTest
	session.Status = onboardingStatusInProgress
	session.LastError = ""
	if err := a.DB.Save(session).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to save onboarding progress", nil, "")
	}

	return r.SendEnvelope(map[string]any{
		"session": a.onboardingSessionToResponse(session),
	})
}

// FinalizeWhatsAppOnboarding runs the connection test and computes the final readiness state.
func (a *App) FinalizeWhatsAppOnboarding(r *fastglue.Request) error {
	session, err := a.resolveOnboardingSessionByID(r)
	if err != nil {
		return nil
	}

	account, err := a.getOnboardingAccountModel(session)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}

	connectionResult, err := a.runAccountConnectionCheck(account)
	if err != nil {
		a.Log.Error("Failed to run onboarding connection test", "error", err, "session_id", session.ID)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to finalize onboarding", nil, "")
	}

	connectionStep := a.getOnboardingStepState(session, onboardingStepConnectionTest)
	if connectionResult.Success {
		connectionStep.Status = onboardingStateCompleted
		connectionStep.Summary = "Connection test passed."
	} else {
		connectionStep.Status = onboardingStateActionRequired
		connectionStep.Summary = firstNonEmpty(connectionResult.Error, "Connection test failed.")
	}
	connectionStep.UpdatedAt = nowRFC3339()
	connectionStep.Details = mergeMaps(connectionStep.Details, map[string]any{
		"success":                  connectionResult.Success,
		"display_phone_number":     connectionResult.DisplayPhoneNumber,
		"verified_name":            connectionResult.VerifiedName,
		"quality_rating":           connectionResult.QualityRating,
		"messaging_limit_tier":     connectionResult.MessagingLimitTier,
		"code_verification_status": connectionResult.CodeVerificationStatus,
		"account_mode":             connectionResult.AccountMode,
		"is_test_number":           connectionResult.IsTestNumber,
		"warning":                  connectionResult.Warning,
		"error":                    connectionResult.Error,
	})
	a.setOnboardingStepState(session, onboardingStepConnectionTest, connectionStep)

	waAccount := a.toWhatsAppAccount(account)
	phoneStatus, _ := a.WhatsApp.GetPhoneNumberStatus(context.Background(), waAccount)
	a.applyPhoneStatusToSession(session, phoneStatus)

	readiness := a.buildOnboardingReadiness(session, connectionResult, phoneStatus)
	if readiness.Status == onboardingStatusReady {
		now := time.Now().UTC()
		session.CompletedAt = &now
	} else {
		session.CompletedAt = nil
	}
	a.setOnboardingReadiness(session, readiness)

	finalStep := a.getOnboardingStepState(session, onboardingStepFinalStatus)
	switch readiness.Status {
	case onboardingStatusReady:
		finalStep.Status = onboardingStateCompleted
	case onboardingStatusWaitingOnMeta:
		finalStep.Status = onboardingStateWaitingOnMeta
	default:
		finalStep.Status = onboardingStateActionRequired
	}
	finalStep.Summary = readiness.Summary
	finalStep.UpdatedAt = nowRFC3339()
	finalStep.Details = map[string]any{
		"readiness": readiness,
	}
	a.setOnboardingStepState(session, onboardingStepFinalStatus, finalStep)

	session.CurrentStep = onboardingStepFinalStatus
	session.Status = readiness.Status
	session.LastError = ""
	if err := a.DB.Save(session).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to finalize onboarding", nil, "")
	}

	return r.SendEnvelope(map[string]any{
		"session": a.onboardingSessionToResponse(session),
	})
}
