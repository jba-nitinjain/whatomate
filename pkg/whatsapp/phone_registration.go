package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// PhoneNumberStatus represents the current state of a WhatsApp business phone number.
type PhoneNumberStatus struct {
	DisplayPhoneNumber     string `json:"display_phone_number"`
	VerifiedName           string `json:"verified_name"`
	CodeVerificationStatus string `json:"code_verification_status"`
	AccountMode            string `json:"account_mode"`
	QualityRating          string `json:"quality_rating"`
	MessagingLimitTier     string `json:"messaging_limit_tier"`
}

// RequestCodeResponse represents a successful response from Meta's request_code endpoint.
type RequestCodeResponse struct {
	Success bool `json:"success"`
}

// VerifyCodeResponse represents a successful response from Meta's verify_code endpoint.
type VerifyCodeResponse struct {
	Success bool   `json:"success"`
	ID      string `json:"id,omitempty"`
}

// RegisterPhoneNumberResponse represents a successful response from Meta's register endpoint.
type RegisterPhoneNumberResponse struct {
	Success bool `json:"success"`
}

// TwoStepVerificationResponse represents a successful response from updating the two-step PIN.
type TwoStepVerificationResponse struct {
	Success bool `json:"success"`
}

// BusinessAccountBackup contains optional migration data for phone registration.
type BusinessAccountBackup struct {
	Password string `json:"password,omitempty"`
	Data     string `json:"data,omitempty"`
}

// RegisterPhoneNumberRequest contains the registration payload expected by Meta.
type RegisterPhoneNumberRequest struct {
	MessagingProduct          string                 `json:"messaging_product"`
	Pin                       string                 `json:"pin"`
	Backup                    *BusinessAccountBackup `json:"backup,omitempty"`
	DataLocalizationRegion    string                 `json:"data_localization_region,omitempty"`
	MetaStoreRetentionMinutes *int                   `json:"meta_store_retention_minutes,omitempty"`
}

// GetPhoneNumberStatus retrieves phone number metadata needed for verification and registration flows.
func (c *Client) GetPhoneNumberStatus(ctx context.Context, account *Account) (*PhoneNumberStatus, error) {
	fields := "display_phone_number,verified_name,code_verification_status,account_mode,quality_rating,messaging_limit_tier"
	url := fmt.Sprintf("%s/%s/%s?fields=%s", c.getBaseURL(), account.APIVersion, account.PhoneID, fields)

	respBody, err := c.doRequest(ctx, http.MethodGet, url, nil, account.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get phone number status: %w", err)
	}

	var status PhoneNumberStatus
	if err := json.Unmarshal(respBody, &status); err != nil {
		return nil, fmt.Errorf("failed to parse phone number status: %w", err)
	}

	return &status, nil
}

// RequestPhoneNumberVerificationCode asks Meta to send a verification code to the configured phone number.
func (c *Client) RequestPhoneNumberVerificationCode(ctx context.Context, account *Account, codeMethod, language string) error {
	url := fmt.Sprintf("%s/%s/%s/request_code", c.getBaseURL(), account.APIVersion, account.PhoneID)
	payload := map[string]string{
		"code_method": codeMethod,
		"language":    language,
	}

	respBody, err := c.doRequest(ctx, http.MethodPost, url, payload, account.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to request verification code: %w", err)
	}

	var resp RequestCodeResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return fmt.Errorf("failed to parse verification code response: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("verification code request was not successful")
	}

	c.Log.Info("Requested phone number verification code", "phone_id", account.PhoneID, "code_method", codeMethod)
	return nil
}

// VerifyPhoneNumberVerificationCode submits the verification code received by SMS/voice to Meta.
func (c *Client) VerifyPhoneNumberVerificationCode(ctx context.Context, account *Account, code string) (*VerifyCodeResponse, error) {
	url := fmt.Sprintf("%s/%s/%s/verify_code", c.getBaseURL(), account.APIVersion, account.PhoneID)
	payload := map[string]string{
		"code": code,
	}

	respBody, err := c.doRequest(ctx, http.MethodPost, url, payload, account.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify phone verification code: %w", err)
	}

	var resp VerifyCodeResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse verify code response: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("verification code confirmation was not successful")
	}

	c.Log.Info("Verified phone number verification code", "phone_id", account.PhoneID)
	return &resp, nil
}

// RegisterPhoneNumber enables messaging on the phone number and sets the initial two-step PIN.
func (c *Client) RegisterPhoneNumber(ctx context.Context, account *Account, input RegisterPhoneNumberRequest) error {
	if input.MessagingProduct == "" {
		input.MessagingProduct = "whatsapp"
	}

	url := fmt.Sprintf("%s/%s/%s/register", c.getBaseURL(), account.APIVersion, account.PhoneID)
	respBody, err := c.doRequest(ctx, http.MethodPost, url, input, account.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to register phone number: %w", err)
	}

	var resp RegisterPhoneNumberResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return fmt.Errorf("failed to parse phone registration response: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("phone registration was not successful")
	}

	c.Log.Info("Registered WhatsApp phone number", "phone_id", account.PhoneID)
	return nil
}

// UpdateTwoStepVerificationPIN sets or rotates the two-step verification PIN for the phone number.
func (c *Client) UpdateTwoStepVerificationPIN(ctx context.Context, account *Account, pin string) error {
	url := fmt.Sprintf("%s/%s/%s", c.getBaseURL(), account.APIVersion, account.PhoneID)
	payload := map[string]string{
		"pin": pin,
	}

	respBody, err := c.doRequest(ctx, http.MethodPost, url, payload, account.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to update two-step verification PIN: %w", err)
	}

	var resp TwoStepVerificationResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return fmt.Errorf("failed to parse two-step verification response: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("two-step verification update was not successful")
	}

	c.Log.Info("Updated WhatsApp two-step verification PIN", "phone_id", account.PhoneID)
	return nil
}
