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

func TestClient_SubscribeApp_WithOverride(t *testing.T) {
	t.Parallel()

	var received map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v21.0/987654321/subscribed_apps", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}))
	defer server.Close()

	client := whatsapp.NewWithTimeout(testutil.NopLogger(), 5*time.Second)
	client.HTTPClient = &http.Client{Transport: &testServerTransport{serverURL: server.URL}}

	err := client.SubscribeApp(testutil.TestContext(t), testAccount(server.URL), &whatsapp.SubscribeAppOptions{
		OverrideCallbackURI: "https://example.com/api/webhook",
		VerifyToken:         "verify-abc",
	})

	require.NoError(t, err)
	assert.Equal(t, "https://example.com/api/webhook", received["override_callback_uri"])
	assert.Equal(t, "verify-abc", received["verify_token"])
}

func TestClient_SubscribeApp_NoOverrideSendsEmptyBody(t *testing.T) {
	t.Parallel()

	var hadBody bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v21.0/987654321/subscribed_apps", r.URL.Path)
		hadBody = r.ContentLength > 0

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}))
	defer server.Close()

	client := whatsapp.NewWithTimeout(testutil.NopLogger(), 5*time.Second)
	client.HTTPClient = &http.Client{Transport: &testServerTransport{serverURL: server.URL}}

	// nil opts and partial opts both fall back to the app-level webhook (no body).
	require.NoError(t, client.SubscribeApp(testutil.TestContext(t), testAccount(server.URL), nil))
	assert.False(t, hadBody)

	require.NoError(t, client.SubscribeApp(testutil.TestContext(t), testAccount(server.URL), &whatsapp.SubscribeAppOptions{
		OverrideCallbackURI: "https://example.com/api/webhook",
	}))
	assert.False(t, hadBody)
}
