package handlers

import (
	"context"
	"regexp"
	"strings"

	"github.com/nikyjain/whatomate/pkg/whatsapp"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

var sixDigitPINPattern = regexp.MustCompile(`^\d{6}$`)

type accountPhoneStatusResponse struct {
	DisplayPhoneNumber        string `json:"display_phone_number"`
	VerifiedName              string `json:"verified_name"`
	CodeVerificationStatus    string `json:"code_verification_status"`
	AccountMode               string `json:"account_mode"`
	QualityRating             string `json:"quality_rating"`
	MessagingLimitTier        string `json:"messaging_limit_tier"`
	IsTestNumber              bool   `json:"is_test_number"`
	Warning                   string `json:"warning,omitempty"`
	TwoStepDisableSupported   bool   `json:"two_step_disable_supported"`
}

type accountPhoneRequestCodeRequest struct {
	CodeMethod string `json:"code_method"`
	Language   string `json:"language"`
}

type accountPhoneVerifyCodeRequest struct {
	Code string `json:"code"`
}

type accountPhoneRegisterRequest struct {
	Pin                       string `json:"pin"`
	BackupPassword            string `json:"backup_password"`
	BackupData                string `json:"backup_data"`
	DataLocalizationRegion    string `json:"data_localization_region"`
	MetaStoreRetentionMinutes *int   `json:"meta_store_retention_minutes"`
}

type accountTwoStepVerificationRequest struct {
	Pin string `json:"pin"`
}

// GetAccountPhoneStatus returns the current phone verification and registration state from Meta.
func (a *App) GetAccountPhoneStatus(r *fastglue.Request) error {
	waAccount, err := a.resolveWhatsAppAccountForPhoneSetup(r)
	if err != nil {
		return nil
	}

	status, err := a.WhatsApp.GetPhoneNumberStatus(context.Background(), waAccount)
	if err != nil {
		a.Log.Error("Failed to get phone number status", "error", err, "phone_id", waAccount.PhoneID)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}

	isTestNumber := status.AccountMode == "SANDBOX" || strings.EqualFold(status.VerifiedName, "Test Number")

	return r.SendEnvelope(accountPhoneStatusResponse{
		DisplayPhoneNumber:      status.DisplayPhoneNumber,
		VerifiedName:            status.VerifiedName,
		CodeVerificationStatus:  status.CodeVerificationStatus,
		AccountMode:             status.AccountMode,
		QualityRating:           status.QualityRating,
		MessagingLimitTier:      status.MessagingLimitTier,
		IsTestNumber:            isTestNumber,
		Warning:                 buildPhoneNumberStatusWarning(status, isTestNumber),
		TwoStepDisableSupported: false,
	})
}

// RequestAccountPhoneVerificationCode asks Meta to send the phone verification code by SMS or voice.
func (a *App) RequestAccountPhoneVerificationCode(r *fastglue.Request) error {
	waAccount, err := a.resolveWhatsAppAccountForPhoneSetup(r)
	if err != nil {
		return nil
	}

	var req accountPhoneRequestCodeRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
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
		a.Log.Error("Failed to request phone verification code", "error", err, "phone_id", waAccount.PhoneID, "code_method", codeMethod)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}

	return r.SendEnvelope(map[string]any{
		"success": true,
		"message": "Verification code requested successfully.",
	})
}

// VerifyAccountPhoneCode confirms the phone verification code received from Meta.
func (a *App) VerifyAccountPhoneCode(r *fastglue.Request) error {
	waAccount, err := a.resolveWhatsAppAccountForPhoneSetup(r)
	if err != nil {
		return nil
	}

	var req accountPhoneVerifyCodeRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	code := strings.TrimSpace(req.Code)
	if code == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Verification code is required", nil, "")
	}

	result, err := a.WhatsApp.VerifyPhoneNumberVerificationCode(context.Background(), waAccount, code)
	if err != nil {
		a.Log.Error("Failed to verify phone code", "error", err, "phone_id", waAccount.PhoneID)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}

	return r.SendEnvelope(map[string]any{
		"success": result.Success,
		"id":      result.ID,
		"message": "Verification code confirmed successfully.",
	})
}

// RegisterAccountPhone enables messaging on the phone number and sets the initial PIN.
func (a *App) RegisterAccountPhone(r *fastglue.Request) error {
	waAccount, err := a.resolveWhatsAppAccountForPhoneSetup(r)
	if err != nil {
		return nil
	}

	var req accountPhoneRegisterRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
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
		Pin:                       req.Pin,
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
		a.Log.Error("Failed to register phone number", "error", err, "phone_id", waAccount.PhoneID)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}

	return r.SendEnvelope(map[string]any{
		"success": true,
		"message": "Phone number registered successfully. Subscribe the app to webhooks if you have not done that yet.",
	})
}

// UpdateAccountTwoStepVerification sets or rotates the 6-digit two-step PIN.
func (a *App) UpdateAccountTwoStepVerification(r *fastglue.Request) error {
	waAccount, err := a.resolveWhatsAppAccountForPhoneSetup(r)
	if err != nil {
		return nil
	}

	var req accountTwoStepVerificationRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	if !isSixDigitPIN(req.Pin) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "PIN must be exactly 6 digits", nil, "")
	}

	if err := a.WhatsApp.UpdateTwoStepVerificationPIN(context.Background(), waAccount, req.Pin); err != nil {
		a.Log.Error("Failed to update two-step verification PIN", "error", err, "phone_id", waAccount.PhoneID)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}

	return r.SendEnvelope(map[string]any{
		"success":                     true,
		"two_step_disable_supported": false,
		"message":                     "Two-step verification PIN updated successfully.",
	})
}

func (a *App) resolveWhatsAppAccountForPhoneSetup(r *fastglue.Request) (*whatsapp.Account, error) {
	orgID, err := a.getOrgID(r)
	if err != nil {
		_ = r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
		return nil, errEnvelopeSent
	}

	id, err := parsePathUUID(r, "id", "account")
	if err != nil {
		return nil, err
	}

	account, err := a.resolveWhatsAppAccountByID(r, id, orgID)
	if err != nil {
		return nil, err
	}

	return a.toWhatsAppAccount(account), nil
}

func normalizePhoneVerificationMethod(method string) string {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "", "SMS":
		return "SMS"
	case "VOICE":
		return "VOICE"
	default:
		return ""
	}
}

func isSixDigitPIN(pin string) bool {
	return sixDigitPINPattern.MatchString(strings.TrimSpace(pin))
}

func buildPhoneNumberStatusWarning(status *whatsapp.PhoneNumberStatus, isTestNumber bool) string {
	if status == nil {
		return ""
	}
	if isTestNumber {
		return "This is a Meta test or sandbox number. It is not suitable for production messaging."
	}
	switch status.CodeVerificationStatus {
	case "NOT_VERIFIED":
		return "This number is not verified with Meta yet. Request and verify a code, then register the number before sending messages."
	case "EXPIRED":
		return "Phone verification has expired. Request and verify a fresh code before registering or sending messages."
	default:
		return ""
	}
}
