package handlers_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/config"
	"github.com/nikyjain/whatomate/internal/handlers"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

const leadTestEncKey = "01234567890123456789012345678901"

func signLeadBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func leadWebhookBody(pageID, leadgenID string) []byte {
	return []byte(fmt.Sprintf(`{"object":"page","entry":[{"id":%q,"time":1729780000,"changes":[{"field":"leadgen","value":{"leadgen_id":%q,"page_id":%q,"form_id":"form_123","ad_id":"ad_456","created_time":1729780000}}]}]}`,
		pageID, leadgenID, pageID))
}

func newLeadWebhookRequest(body []byte, signature string) *fastglue.Request {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetBody(body)
	if signature != "" {
		ctx.Request.Header.Set("X-Hub-Signature-256", signature)
	}
	return &fastglue.Request{RequestCtx: ctx}
}

// createLeadSource stores a source the same way CreateLeadSource does (encrypted secrets).
func createLeadSource(t *testing.T, app *handlers.App, orgID uuid.UUID, appSecret string) *models.LeadSource {
	t.Helper()
	source := &models.LeadSource{
		BaseModel:          models.BaseModel{ID: uuid.New()},
		OrganizationID:     orgID,
		Name:               "Test Page",
		PageID:             "page-" + uuid.NewString()[:12],
		AppSecret:          appSecret,
		PageAccessToken:    "token123",
		WebhookVerifyToken: uuid.NewString(),
		IsActive:           true,
	}
	require.NoError(t, source.EncryptSecrets(leadTestEncKey))
	require.NoError(t, app.DB.Create(source).Error)
	return source
}

func newLeadTestApp(t *testing.T) *handlers.App {
	app := newTestApp(t)
	app.Config.App = config.AppConfig{EncryptionKey: leadTestEncKey}
	return app
}

func countLeads(app *handlers.App, leadgenID string) int64 {
	var n int64
	app.DB.Model(&models.Lead{}).Where("leadgen_id = ?", leadgenID).Count(&n)
	return n
}

func TestLeadWebhookHandler_ValidSignatureQueuesLeadOnce(t *testing.T) {
	app := newLeadTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	source := createLeadSource(t, app, org.ID, "secret123")
	leadgenID := "lead-" + uuid.NewString()[:12]
	body := leadWebhookBody(source.PageID, leadgenID)
	sig := signLeadBody("secret123", body)

	req := newLeadWebhookRequest(body, sig)
	require.NoError(t, app.LeadWebhookHandler(req))
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var lead models.Lead
	require.NoError(t, app.DB.Where("leadgen_id = ?", leadgenID).First(&lead).Error)
	assert.Equal(t, org.ID, lead.OrganizationID)
	assert.Equal(t, source.ID, lead.LeadSourceID)
	assert.Equal(t, models.StatusLeadReceived, lead.Status)

	// Meta redelivery must not create a second row.
	require.NoError(t, app.LeadWebhookHandler(newLeadWebhookRequest(body, sig)))
	assert.Equal(t, int64(1), countLeads(app, leadgenID))
}

func TestLeadWebhookHandler_RejectsBadSignature(t *testing.T) {
	app := newLeadTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	source := createLeadSource(t, app, org.ID, "secret123")
	leadgenID := "lead-" + uuid.NewString()[:12]
	body := leadWebhookBody(source.PageID, leadgenID)

	req := newLeadWebhookRequest(body, signLeadBody("wrong-secret", body))
	_ = app.LeadWebhookHandler(req)
	assert.Equal(t, fasthttp.StatusForbidden, testutil.GetResponseStatusCode(req))
	assert.Zero(t, countLeads(app, leadgenID))
}

func TestLeadWebhookHandler_FailsClosedWithoutAppSecret(t *testing.T) {
	app := newLeadTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	source := createLeadSource(t, app, org.ID, "")
	leadgenID := "lead-" + uuid.NewString()[:12]
	body := leadWebhookBody(source.PageID, leadgenID)

	// An HMAC keyed with the empty string is computable by anyone; refuse it.
	req := newLeadWebhookRequest(body, signLeadBody("", body))
	_ = app.LeadWebhookHandler(req)
	assert.Equal(t, fasthttp.StatusForbidden, testutil.GetResponseStatusCode(req))
	assert.Zero(t, countLeads(app, leadgenID))
}

func TestLeadWebhookVerify(t *testing.T) {
	app := newLeadTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	source := createLeadSource(t, app, org.ID, "secret123")

	ok := testutil.NewGETRequest(t)
	testutil.SetQueryParam(ok, "hub.mode", "subscribe")
	testutil.SetQueryParam(ok, "hub.verify_token", source.WebhookVerifyToken)
	testutil.SetQueryParam(ok, "hub.challenge", "challenge_source")
	require.NoError(t, app.LeadWebhookVerify(ok))
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(ok))
	assert.Equal(t, "challenge_source", string(testutil.GetResponseBody(ok)))

	bad := testutil.NewGETRequest(t)
	testutil.SetQueryParam(bad, "hub.mode", "subscribe")
	testutil.SetQueryParam(bad, "hub.verify_token", "wrong_token")
	_ = app.LeadWebhookVerify(bad)
	assert.Equal(t, fasthttp.StatusForbidden, testutil.GetResponseStatusCode(bad))
}

// The checks below run before any database or cache access, so they need no
// external services and always run.

func TestLeadWebhookHandler_RejectsMissingSignature(t *testing.T) {
	app := &handlers.App{Log: testutil.NopLogger(), Config: &config.Config{}}
	req := newLeadWebhookRequest(leadWebhookBody("123", "lead_1"), "")
	_ = app.LeadWebhookHandler(req)
	assert.Equal(t, fasthttp.StatusForbidden, testutil.GetResponseStatusCode(req))
}

func TestLeadWebhookHandler_RejectsNonPageObject(t *testing.T) {
	app := &handlers.App{Log: testutil.NopLogger(), Config: &config.Config{}}
	req := newLeadWebhookRequest([]byte(`{"object":"user"}`), "sha256=00")
	_ = app.LeadWebhookHandler(req)
	assert.Equal(t, fasthttp.StatusBadRequest, testutil.GetResponseStatusCode(req))
}
