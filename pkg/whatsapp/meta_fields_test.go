package whatsapp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/nikyjain/whatomate/pkg/whatsapp"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fieldRejectingServer mimics a Graph API that does not know is_on_biz_app /
// platform_type: requests asking for them fail with (#100) nonexisting field.
type fieldRejectingServer struct {
	mu     sync.Mutex
	fields []string
	srv    *httptest.Server
}

func newFieldRejectingServer(t *testing.T, rejectNew bool) *fieldRejectingServer {
	t.Helper()
	s := &fieldRejectingServer{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fields := r.URL.Query().Get("fields")
		s.mu.Lock()
		s.fields = append(s.fields, r.URL.Path+"?"+fields)
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if rejectNew && (strings.Contains(fields, "is_on_biz_app") || strings.Contains(fields, "platform_type")) {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
				"message": "(#100) Tried accessing nonexisting field (is_on_biz_app) on node type (WhatsAppBusinessPhoneNumber)",
				"type":    "OAuthException",
				"code":    100,
			}})
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/phone_numbers"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": "123456789"}}})
		case strings.HasSuffix(r.URL.Path, "/987654321"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "987654321", "name": "Biz"})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"display_phone_number":     "+1 555 0100",
				"verified_name":            "Acme",
				"code_verification_status": "VERIFIED",
				"account_mode":             "LIVE",
				"quality_rating":           "GREEN",
				"messaging_limit_tier":     "TIER_1K",
			})
		}
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *fieldRejectingServer) requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.fields...)
}

func TestGetPhoneNumberStatus_RetriesWithLegacyFields(t *testing.T) {
	t.Parallel()
	s := newFieldRejectingServer(t, true)
	client := whatsapp.NewWithBaseURL(testutil.NopLogger(), s.srv.URL)

	status, err := client.GetPhoneNumberStatus(context.Background(), testAccount(s.srv.URL))
	require.NoError(t, err)
	assert.Equal(t, "Acme", status.VerifiedName)
	assert.Equal(t, "TIER_1K", status.MessagingLimitTier)

	reqs := s.requests()
	require.Len(t, reqs, 2)
	assert.Contains(t, reqs[0], "is_on_biz_app")
	assert.True(t, strings.HasSuffix(reqs[1], "?"+whatsapp.PhoneStatusFieldsLegacy), reqs[1])
}

func TestGetPhoneNumberStatus_NoRetryWhenFieldsAccepted(t *testing.T) {
	t.Parallel()
	s := newFieldRejectingServer(t, false)
	client := whatsapp.NewWithBaseURL(testutil.NopLogger(), s.srv.URL)

	_, err := client.GetPhoneNumberStatus(context.Background(), testAccount(s.srv.URL))
	require.NoError(t, err)
	assert.Len(t, s.requests(), 1)
}

func TestValidateCredentials_RetriesWithLegacyFields(t *testing.T) {
	t.Parallel()
	s := newFieldRejectingServer(t, true)
	client := whatsapp.NewWithBaseURL(testutil.NopLogger(), s.srv.URL)

	result, err := client.ValidateCredentials(context.Background(), "123456789", "987654321", "token", "v21.0")
	require.NoError(t, err)
	assert.Equal(t, "Acme", result.VerifiedName)
	assert.Contains(t, s.requests()[1], "?"+whatsapp.PhoneValidateFieldsLegacy)
}

func TestGetPhoneNumberInfo_RetriesWithLegacyFields(t *testing.T) {
	t.Parallel()
	s := newFieldRejectingServer(t, true)
	client := whatsapp.NewWithBaseURL(testutil.NopLogger(), s.srv.URL)

	info, err := client.GetPhoneNumberInfo(context.Background(), "123456789", "token", "v21.0")
	require.NoError(t, err)
	assert.Equal(t, "Acme", info.VerifiedName)
}

func TestIsUnknownFieldError(t *testing.T) {
	t.Parallel()
	body := []byte(`{"error":{"message":"(#100) Tried accessing nonexisting field (platform_type)","code":100}}`)
	assert.True(t, whatsapp.IsUnknownFieldResponse(http.StatusBadRequest, body))
	assert.False(t, whatsapp.IsUnknownFieldResponse(http.StatusOK, body))
	other := []byte(`{"error":{"message":"Invalid OAuth access token","code":190}}`)
	assert.False(t, whatsapp.IsUnknownFieldResponse(http.StatusUnauthorized, other))
}
