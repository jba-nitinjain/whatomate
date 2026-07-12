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

func TestClient_SendInteractiveReplyButtonsWithHeader(t *testing.T) {
	t.Parallel()

	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"messages": []map[string]string{{"id": "wamid.1"}}})
	}))
	defer server.Close()

	client := whatsapp.NewWithTimeout(testutil.NopLogger(), 5*time.Second)
	client.HTTPClient = &http.Client{Transport: &testServerTransport{serverURL: server.URL}}

	_, err := client.SendInteractiveReplyButtonsWithHeader(
		testutil.TestContext(t), testAccount(server.URL), "1234567890",
		"Join our event", []whatsapp.Button{{ID: "yes", Title: "Attending"}},
		"image", "https://example.com/poster.jpg",
	)
	require.NoError(t, err)

	interactive, _ := body["interactive"].(map[string]any)
	require.NotNil(t, interactive)
	header, _ := interactive["header"].(map[string]any)
	require.NotNil(t, header, "media header should be present")
	assert.Equal(t, "image", header["type"])
	img, _ := header["image"].(map[string]any)
	require.NotNil(t, img)
	assert.Equal(t, "https://example.com/poster.jpg", img["link"])
}

func TestClient_SendInteractiveReplyButtons_NoHeaderByDefault(t *testing.T) {
	t.Parallel()

	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"messages": []map[string]string{{"id": "wamid.1"}}})
	}))
	defer server.Close()

	client := whatsapp.NewWithTimeout(testutil.NopLogger(), 5*time.Second)
	client.HTTPClient = &http.Client{Transport: &testServerTransport{serverURL: server.URL}}

	_, err := client.SendInteractiveReplyButtons(
		testutil.TestContext(t), testAccount(server.URL), "1234567890",
		"Pick one", []whatsapp.Button{{ID: "a", Title: "A"}},
	)
	require.NoError(t, err)

	interactive, _ := body["interactive"].(map[string]any)
	require.NotNil(t, interactive)
	_, hasHeader := interactive["header"]
	assert.False(t, hasHeader, "no header should be sent when none configured")
}
