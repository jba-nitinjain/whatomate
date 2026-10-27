package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/pkg/whatsapp"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

// Coexistence (WhatsApp Business app) numbers are already registered, so the
// legacy register-phone endpoint must not call Meta's /register for them.
func TestApp_RegisterAccountPhone_SkipsMetaForCoexistence(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	user := createAdminUser(t, app, org.ID)

	account := &models.WhatsAppAccount{
		OrganizationID: org.ID,
		Name:           "Coexistence Account",
		PhoneID:        "coex-phone-1",
		BusinessID:     "coex-waba-1",
		AccessToken:    "test_token",
		APIVersion:     "v21.0",
		Status:         "active",
		IsSMB:          true,
	}
	require.NoError(t, app.DB.Create(account).Error)

	var metaCalls atomic.Int32
	metaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metaCalls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer metaServer.Close()
	app.WhatsApp = whatsapp.NewWithBaseURL(app.Log, metaServer.URL)

	req := testutil.NewJSONRequest(t, map[string]any{"pin": "123456"})
	testutil.SetAuthContext(req, org.ID, user.ID)
	testutil.SetPathParam(req, "id", account.ID.String())

	require.NoError(t, app.RegisterAccountPhone(req))
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))
	assert.Contains(t, string(testutil.GetResponseBody(req)), `"skipped":true`)
	assert.Equal(t, int32(0), metaCalls.Load(), "Meta /register must not be called for coexistence numbers")
}
