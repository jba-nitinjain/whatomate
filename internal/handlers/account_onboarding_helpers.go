package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/crypto"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/pkg/whatsapp"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

func (a *App) resolveOnboardingSessionByID(r *fastglue.Request) (*models.WhatsAppOnboardingSession, error) {
	orgID, err := a.getOrgID(r)
	if err != nil {
		_ = r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
		return nil, errEnvelopeSent
	}
	id, err := parsePathUUID(r, "id", "session")
	if err != nil {
		return nil, err
	}

	var session models.WhatsAppOnboardingSession
	if err := a.DB.Where("id = ? AND organization_id = ?", id, orgID).First(&session).Error; err != nil {
		_ = r.SendErrorEnvelope(fasthttp.StatusNotFound, "Onboarding session not found", nil, "")
		return nil, errEnvelopeSent
	}
	return &session, nil
}

func (a *App) getAllOnboardingStepStates(session *models.WhatsAppOnboardingSession) map[string]OnboardingStepState {
	states := defaultOnboardingStepStates()
	if len(session.StepState) == 0 {
		return states
	}

	var stored map[string]OnboardingStepState
	if err := decodeJSONB(session.StepState, &stored); err == nil {
		for key, state := range stored {
			states[key] = state
		}
	}
	return states
}

func (a *App) getOnboardingStepState(session *models.WhatsAppOnboardingSession, stepName string) OnboardingStepState {
	return a.getAllOnboardingStepStates(session)[stepName]
}

func (a *App) setOnboardingStepState(session *models.WhatsAppOnboardingSession, stepName string, step OnboardingStepState) {
	states := a.getAllOnboardingStepStates(session)
	states[stepName] = step
	if encoded, err := structToJSONB(states); err == nil {
		session.StepState = encoded
	}
}

func (a *App) getOnboardingReadiness(session *models.WhatsAppOnboardingSession) OnboardingReadinessStatus {
	if len(session.Readiness) == 0 {
		return OnboardingReadinessStatus{}
	}

	var readiness OnboardingReadinessStatus
	if err := decodeJSONB(session.Readiness, &readiness); err != nil {
		return OnboardingReadinessStatus{}
	}
	return readiness
}

func (a *App) setOnboardingReadiness(session *models.WhatsAppOnboardingSession, readiness OnboardingReadinessStatus) {
	if encoded, err := structToJSONB(readiness); err == nil {
		session.Readiness = encoded
	}
}

func (a *App) getOnboardingAccountModel(session *models.WhatsAppOnboardingSession) (*models.WhatsAppAccount, error) {
	if session.AccountID == nil || *session.AccountID == uuid.Nil {
		return nil, fmt.Errorf("no WhatsApp account has been imported into this onboarding session yet")
	}

	var account models.WhatsAppAccount
	if err := a.DB.Where("id = ? AND organization_id = ?", *session.AccountID, session.OrganizationID).First(&account).Error; err != nil {
		return nil, fmt.Errorf("the onboarding account could not be loaded")
	}
	a.decryptAccountSecrets(&account)
	return &account, nil
}

func (a *App) resolveOnboardingWhatsAppAccount(session *models.WhatsAppOnboardingSession) (*whatsapp.Account, *models.WhatsAppAccount, error) {
	account, err := a.getOnboardingAccountModel(session)
	if err == nil {
		return a.toWhatsAppAccount(account), account, nil
	}

	crypto.DecryptFields(a.Config.App.EncryptionKey, &session.AccessToken, &session.AppSecret)
	if strings.TrimSpace(session.PhoneID) == "" || strings.TrimSpace(session.BusinessID) == "" || strings.TrimSpace(session.AccessToken) == "" {
		return nil, nil, err
	}

	return &whatsapp.Account{
		PhoneID:     session.PhoneID,
		BusinessID:  session.BusinessID,
		AppID:       session.AppID,
		APIVersion:  onboardingAPIVersion(session.APIVersion, nil),
		AccessToken: session.AccessToken,
	}, nil, nil
}

func (a *App) upsertWhatsAppAccountFromOnboarding(session *models.WhatsAppOnboardingSession, input onboardingAccountImportInput) (*models.WhatsAppAccount, error) {
	var account models.WhatsAppAccount
	if session.AccountID != nil && *session.AccountID != uuid.Nil {
		_ = a.DB.Where("id = ? AND organization_id = ?", *session.AccountID, session.OrganizationID).First(&account).Error
	}
	if account.ID == uuid.Nil {
		_ = a.DB.Where("organization_id = ? AND phone_id = ?", session.OrganizationID, input.PhoneID).First(&account).Error
	}

	encToken, err := crypto.Encrypt(input.AccessToken, a.Config.App.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt access token: %w", err)
	}
	encSecret, err := crypto.Encrypt(input.AppSecret, a.Config.App.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt app secret: %w", err)
	}

	requestedName := firstNonEmpty(strings.TrimSpace(input.AccountName), session.AccountName, formatFallbackAccountName(input.PhoneID))
	if account.ID == uuid.Nil {
		account = models.WhatsAppAccount{
			OrganizationID: session.OrganizationID,
			Status:         "active",
		}
	}
	account.Name = a.uniqueOnboardingAccountName(session.OrganizationID, account.ID, requestedName, input.PhoneID)
	account.AppID = strings.TrimSpace(input.AppID)
	account.PhoneID = strings.TrimSpace(input.PhoneID)
	account.BusinessID = strings.TrimSpace(input.BusinessID)
	account.AccessToken = encToken
	if encSecret != "" {
		account.AppSecret = encSecret
	}
	if strings.TrimSpace(input.WebhookVerifyToken) != "" {
		account.WebhookVerifyToken = strings.TrimSpace(input.WebhookVerifyToken)
	}
	if strings.TrimSpace(account.WebhookVerifyToken) == "" {
		account.WebhookVerifyToken = generateVerifyToken()
	}
	account.APIVersion = onboardingAPIVersion(input.APIVersion, nil)
	account.Status = "active"

	if account.ID == uuid.Nil {
		if err := a.DB.Create(&account).Error; err != nil {
			return nil, fmt.Errorf("failed to create WhatsApp account: %w", err)
		}
		return &account, nil
	}

	if err := a.DB.Save(&account).Error; err != nil {
		return nil, fmt.Errorf("failed to update WhatsApp account: %w", err)
	}
	a.InvalidateWhatsAppAccountCache(account.PhoneID)
	return &account, nil
}

func (a *App) uniqueOnboardingAccountName(orgID, existingID uuid.UUID, requestedName, phoneID string) string {
	base := truncateAccountName(firstNonEmpty(requestedName, formatFallbackAccountName(phoneID)))
	name := base
	suffix := trimForName(phoneID)
	if suffix == "" {
		suffix = uuid.New().String()[:8]
	}

	for i := 0; i < 100; i++ {
		var count int64
		query := a.DB.Model(&models.WhatsAppAccount{}).Where("organization_id = ? AND name = ?", orgID, name)
		if existingID != uuid.Nil {
			query = query.Where("id <> ?", existingID)
		}
		if err := query.Count(&count).Error; err == nil && count == 0 {
			return name
		}

		if i == 0 {
			name = truncateAccountName(fmt.Sprintf("%s (%s)", base, suffix))
			continue
		}
		name = truncateAccountName(fmt.Sprintf("%s (%s-%d)", base, suffix, i+1))
	}

	return truncateAccountName(base + " " + uuid.New().String()[:6])
}

func (a *App) applyOnboardingPreflight(session *models.WhatsAppOnboardingSession, metaCfg *MetaOnboardingConfig) {
	step := a.getOnboardingStepState(session, onboardingStepPreflight)
	step.UpdatedAt = nowRFC3339()
	step.Details = mergeMaps(step.Details, map[string]any{
		"org_permission":     true,
		"meta_configured":    false,
		"meta_app_id":        "",
		"embedded_config_id": "",
		"graph_api_version":  defaultMetaGraphAPIVersion,
		"callback_url":       "",
		"required_scopes":    []string{},
		"https_callback_url": false,
	})

	if metaCfg != nil {
		step.Details["meta_configured"] = metaCfg.IsConfigured
		step.Details["meta_app_id"] = metaCfg.MetaAppID
		step.Details["embedded_config_id"] = metaCfg.EmbeddedSignupConfigID
		step.Details["graph_api_version"] = metaCfg.GraphAPIVersion
		step.Details["callback_url"] = metaCfg.CallbackURL
		step.Details["required_scopes"] = metaCfg.RequiredScopes
		step.Details["https_callback_url"] = strings.HasPrefix(strings.ToLower(metaCfg.CallbackURL), "https://")
	}

	switch {
	case metaCfg == nil || !metaCfg.IsConfigured:
		step.Status = onboardingStateActionRequired
		step.Summary = "A super admin must configure the Meta app, app secret, embedded signup config ID, and public webhook URL first."
	case !strings.HasPrefix(strings.ToLower(metaCfg.CallbackURL), "https://"):
		step.Status = onboardingStateActionRequired
		step.Summary = "The public webhook callback URL must use HTTPS."
	default:
		step.Status = onboardingStateCompleted
		step.Summary = "Platform prerequisites are configured."
	}

	a.setOnboardingStepState(session, onboardingStepPreflight, step)
}

func (a *App) applyPhoneStatusToSession(session *models.WhatsAppOnboardingSession, status *whatsapp.PhoneNumberStatus) {
	step := a.getOnboardingStepState(session, onboardingStepPhoneSetup)
	step.UpdatedAt = nowRFC3339()
	if status == nil {
		a.setOnboardingStepState(session, onboardingStepPhoneSetup, step)
		return
	}

	isTestNumber := status.AccountMode == "SANDBOX" || strings.EqualFold(status.VerifiedName, "Test Number")
	step.Details = mergeMaps(step.Details, map[string]any{
		"display_phone_number":     status.DisplayPhoneNumber,
		"verified_name":            status.VerifiedName,
		"code_verification_status": status.CodeVerificationStatus,
		"account_mode":             status.AccountMode,
		"quality_rating":           status.QualityRating,
		"messaging_limit_tier":     status.MessagingLimitTier,
		"is_test_number":           isTestNumber,
	})

	switch status.CodeVerificationStatus {
	case "VERIFIED":
		step.Status = onboardingStateCompleted
		step.Summary = "Phone number is verified with Meta."
	case "EXPIRED":
		step.Status = onboardingStateActionRequired
		step.Summary = "Phone verification expired. Request and confirm a new code, then register the number again."
	default:
		step.Status = onboardingStateActionRequired
		step.Summary = "Phone number is not verified yet. Request a code, confirm it, then register the number."
	}
	if isTestNumber {
		step.Status = onboardingStateActionRequired
		step.Summary = "Meta reports this as a sandbox or test number. Switch to a production number before final launch."
	}

	a.setOnboardingStepState(session, onboardingStepPhoneSetup, step)
}

func (a *App) setOnboardingFailure(session *models.WhatsAppOnboardingSession, stepName string, err error) {
	step := a.getOnboardingStepState(session, stepName)
	step.Status = onboardingStateActionRequired
	step.Summary = err.Error()
	step.UpdatedAt = nowRFC3339()
	step.Details = mergeMaps(step.Details, map[string]any{
		"error": err.Error(),
	})
	a.setOnboardingStepState(session, stepName, step)
	session.Status = onboardingStatusFailed
	session.CurrentStep = stepName
	session.LastError = err.Error()
}

func (a *App) onboardingSessionToResponse(session *models.WhatsAppOnboardingSession) WhatsAppOnboardingSessionResponse {
	response := WhatsAppOnboardingSessionResponse{
		ID:                 session.ID,
		OrganizationID:     session.OrganizationID,
		AccountID:          session.AccountID,
		Mode:               session.Mode,
		Status:             session.Status,
		CurrentStep:        session.CurrentStep,
		AccountName:        session.AccountName,
		AppID:              session.AppID,
		PhoneID:            session.PhoneID,
		BusinessID:         session.BusinessID,
		WebhookVerifyToken: session.WebhookVerifyToken,
		APIVersion:         session.APIVersion,
		HasAccessToken:     strings.TrimSpace(session.AccessToken) != "",
		HasAppSecret:       strings.TrimSpace(session.AppSecret) != "",
		StepState:          a.getAllOnboardingStepStates(session),
		Readiness:          a.getOnboardingReadiness(session),
		LastError:          session.LastError,
		CreatedAt:          session.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          session.UpdatedAt.Format(time.RFC3339),
	}
	if len(session.Metadata) > 0 {
		response.Metadata = session.Metadata
	}
	if session.CompletedAt != nil {
		response.CompletedAt = session.CompletedAt.UTC().Format(time.RFC3339)
	}
	return response
}

func (a *App) importOnboardingAssets(session *models.WhatsAppOnboardingSession, input onboardingAccountImportInput, metadata map[string]any, mode string) error {
	if strings.TrimSpace(input.PhoneID) == "" || strings.TrimSpace(input.BusinessID) == "" || strings.TrimSpace(input.AccessToken) == "" {
		return fmt.Errorf("phone_id, business_id, and access_token are required")
	}

	session.Mode = normalizeOnboardingMode(mode)
	session.APIVersion = onboardingAPIVersion(input.APIVersion, nil)

	tempAccount := &whatsapp.Account{
		PhoneID:     input.PhoneID,
		BusinessID:  input.BusinessID,
		AppID:       input.AppID,
		APIVersion:  session.APIVersion,
		AccessToken: input.AccessToken,
	}
	phoneStatus, err := a.WhatsApp.GetPhoneNumberStatus(context.Background(), tempAccount)
	if err != nil {
		a.setOnboardingFailure(session, onboardingStepAssetAcquisition, err)
		_ = a.DB.Save(session).Error
		return fmt.Errorf("failed to access the Meta phone number: %w", err)
	}

	if metadata != nil {
		session.Metadata = mergeJSONB(session.Metadata, metadata)
	}

	if metaCfg, cfgErr := a.getMetaOnboardingConfig(); cfgErr == nil {
		a.applyOnboardingPreflight(session, metaCfg)
	}

	account, err := a.upsertWhatsAppAccountFromOnboarding(session, input)
	if err != nil {
		a.setOnboardingFailure(session, onboardingStepAssetImport, err)
		_ = a.DB.Save(session).Error
		return err
	}

	session.AccountID = &account.ID
	session.AccountName = account.Name
	session.AppID = account.AppID
	session.PhoneID = account.PhoneID
	session.BusinessID = account.BusinessID
	session.WebhookVerifyToken = account.WebhookVerifyToken
	session.APIVersion = account.APIVersion
	session.AccessToken = account.AccessToken
	session.AppSecret = account.AppSecret
	session.Metadata = mergeJSONB(session.Metadata, map[string]any{
		"import_mode": session.Mode,
	})

	acquisitionStep := a.getOnboardingStepState(session, onboardingStepAssetAcquisition)
	acquisitionStep.Status = onboardingStateCompleted
	acquisitionStep.Summary = "Meta assets received successfully."
	acquisitionStep.UpdatedAt = nowRFC3339()
	acquisitionStep.Details = mergeMaps(acquisitionStep.Details, map[string]any{
		"token_exchange_status": onboardingStateCompleted,
		"phone_id":              account.PhoneID,
		"business_id":           account.BusinessID,
		"app_id":                account.AppID,
	})
	a.setOnboardingStepState(session, onboardingStepAssetAcquisition, acquisitionStep)

	importStep := a.getOnboardingStepState(session, onboardingStepAssetImport)
	importStep.Status = onboardingStateCompleted
	importStep.Summary = "WhatsApp account imported into Whatomate."
	importStep.UpdatedAt = nowRFC3339()
	importStep.Details = mergeMaps(importStep.Details, map[string]any{
		"account_id":   account.ID.String(),
		"account_name": account.Name,
		"phone_id":     account.PhoneID,
		"business_id":  account.BusinessID,
	})
	a.setOnboardingStepState(session, onboardingStepAssetImport, importStep)

	a.applyPhoneStatusToSession(session, phoneStatus)

	webhookStep := a.getOnboardingStepState(session, onboardingStepWebhooks)
	webhookStep.Summary = "Validate the callback URL and subscribe the app to Meta webhooks."
	webhookStep.Details = mergeMaps(webhookStep.Details, map[string]any{
		"verify_token": account.WebhookVerifyToken,
	})
	a.setOnboardingStepState(session, onboardingStepWebhooks, webhookStep)

	session.CurrentStep = onboardingStepPhoneSetup
	session.Status = onboardingStatusInProgress
	session.LastError = ""
	return a.DB.Save(session).Error
}

func toString(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}
