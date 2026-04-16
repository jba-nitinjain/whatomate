package whatsapp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nikyjain/whatomate/pkg/whatsapp"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_GetPhoneNumberStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v21.0/123456789", r.URL.Path)
		assert.Contains(t, r.URL.RawQuery, "display_phone_number")
		assert.Equal(t, "Bearer test-access-token", r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(whatsapp.PhoneNumberStatus{
			DisplayPhoneNumber:     "+1 555 000 1234",
			VerifiedName:           "Whatomate",
			CodeVerificationStatus: "VERIFIED",
			AccountMode:            "LIVE",
			QualityRating:          "GREEN",
			MessagingLimitTier:     "TIER_1K",
		})
	}))
	defer server.Close()

	log := testutil.NopLogger()
	client := whatsapp.NewWithTimeout(log, 5*time.Second)
	client.HTTPClient = &http.Client{
		Transport: &testServerTransport{serverURL: server.URL},
	}

	status, err := client.GetPhoneNumberStatus(testutil.TestContext(t), testAccount(server.URL))

	require.NoError(t, err)
	assert.Equal(t, "+1 555 000 1234", status.DisplayPhoneNumber)
	assert.Equal(t, "VERIFIED", status.CodeVerificationStatus)
	assert.Equal(t, "LIVE", status.AccountMode)
}

func TestClient_RequestPhoneNumberVerificationCode(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v21.0/123456789/request_code", r.URL.Path)

		var body map[string]string
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)
		assert.Equal(t, "SMS", body["code_method"])
		assert.Equal(t, "en_US", body["language"])

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}))
	defer server.Close()

	log := testutil.NopLogger()
	client := whatsapp.NewWithTimeout(log, 5*time.Second)
	client.HTTPClient = &http.Client{
		Transport: &testServerTransport{serverURL: server.URL},
	}

	err := client.RequestPhoneNumberVerificationCode(testutil.TestContext(t), testAccount(server.URL), "SMS", "en_US")
	require.NoError(t, err)
}

func TestClient_RequestPhoneNumberVerificationCode_PropagatesMetaError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(whatsapp.MetaAPIError{
			Error: struct {
				Message      string `json:"message"`
				Type         string `json:"type"`
				Code         int    `json:"code"`
				ErrorSubcode int    `json:"error_subcode"`
				ErrorUserMsg string `json:"error_user_msg"`
				ErrorData    struct {
					Details string `json:"details"`
				} `json:"error_data"`
				FBTraceID string `json:"fbtrace_id"`
			}{
				Message: "Verification code cannot be sent right now",
				Code:    100,
			},
		})
	}))
	defer server.Close()

	log := testutil.NopLogger()
	client := whatsapp.NewWithTimeout(log, 5*time.Second)
	client.HTTPClient = &http.Client{
		Transport: &testServerTransport{serverURL: server.URL},
	}

	err := client.RequestPhoneNumberVerificationCode(testutil.TestContext(t), testAccount(server.URL), "SMS", "en_US")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Verification code cannot be sent right now")
}

func TestClient_VerifyPhoneNumberVerificationCode(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v21.0/123456789/verify_code", r.URL.Path)

		var body map[string]string
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)
		assert.Equal(t, "482951", body["code"])

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(whatsapp.VerifyCodeResponse{
			Success: true,
			ID:      "123456789",
		})
	}))
	defer server.Close()

	log := testutil.NopLogger()
	client := whatsapp.NewWithTimeout(log, 5*time.Second)
	client.HTTPClient = &http.Client{
		Transport: &testServerTransport{serverURL: server.URL},
	}

	resp, err := client.VerifyPhoneNumberVerificationCode(testutil.TestContext(t), testAccount(server.URL), "482951")

	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, "123456789", resp.ID)
}

func TestClient_RegisterPhoneNumber(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v21.0/123456789/register", r.URL.Path)

		var body map[string]any
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)
		assert.Equal(t, "whatsapp", body["messaging_product"])
		assert.Equal(t, "123456", body["pin"])

		backup, ok := body["backup"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "backup-secret", backup["password"])
		assert.Equal(t, "encrypted-backup", backup["data"])

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}))
	defer server.Close()

	log := testutil.NopLogger()
	client := whatsapp.NewWithTimeout(log, 5*time.Second)
	client.HTTPClient = &http.Client{
		Transport: &testServerTransport{serverURL: server.URL},
	}

	err := client.RegisterPhoneNumber(testutil.TestContext(t), testAccount(server.URL), whatsapp.RegisterPhoneNumberRequest{
		Pin: "123456",
		Backup: &whatsapp.BusinessAccountBackup{
			Password: "backup-secret",
			Data:     "encrypted-backup",
		},
	})

	require.NoError(t, err)
}

func TestClient_UpdateTwoStepVerificationPIN(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v21.0/123456789", r.URL.Path)

		var body map[string]string
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)
		assert.Equal(t, "654321", body["pin"])

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}))
	defer server.Close()

	log := testutil.NopLogger()
	client := whatsapp.NewWithTimeout(log, 5*time.Second)
	client.HTTPClient = &http.Client{
		Transport: &testServerTransport{serverURL: server.URL},
	}

	err := client.UpdateTwoStepVerificationPIN(testutil.TestContext(t), testAccount(server.URL), "654321")
	require.NoError(t, err)
}
