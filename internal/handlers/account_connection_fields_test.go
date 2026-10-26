package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nikyjain/whatomate/internal/config"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/pkg/whatsapp"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Graph API that rejects the newer fields must not make the onboarding
// connection check fail: it retries once with main's original field list.
func TestRunAccountConnectionCheck_RetriesWithoutNewMetaFields(t *testing.T) {
	var mu sync.Mutex
	var phoneFieldRequests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fields := r.URL.Query().Get("fields")
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(fields, "whatsapp_business_manager_messaging_limit") ||
			strings.Contains(fields, "is_on_biz_app") || strings.Contains(fields, "platform_type") {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
				"message": "(#100) Tried accessing nonexisting field (whatsapp_business_manager_messaging_limit)",
				"code":    100,
			}})
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/phone_numbers"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": "phone-1"}}})
		case strings.HasSuffix(r.URL.Path, "/waba-1"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "waba-1", "name": "Biz"})
		default:
			mu.Lock()
			phoneFieldRequests = append(phoneFieldRequests, fields)
			mu.Unlock()
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
	t.Cleanup(server.Close)

	log := testutil.NopLogger()
	cfg := &config.Config{}
	cfg.WhatsApp.BaseURL = server.URL
	app := &App{
		Config:     cfg,
		Log:        log,
		WhatsApp:   whatsapp.NewWithBaseURL(log, server.URL),
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}
	account := &models.WhatsAppAccount{PhoneID: "phone-1", BusinessID: "waba-1", AccessToken: "tok", APIVersion: "v21.0"}

	check, err := app.runAccountConnectionCheck(account)
	require.NoError(t, err)
	require.True(t, check.Success, check.Error)
	assert.Equal(t, "Acme", check.VerifiedName)
	assert.Equal(t, "TIER_1K", check.MessagingLimitTier)
	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, phoneFieldRequests, accountConnectionFieldsLegacy)
}
