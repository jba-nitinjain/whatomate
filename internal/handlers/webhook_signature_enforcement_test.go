package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/config"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

func TestWebhookPayloadPhoneNumberID(t *testing.T) {
	t.Parallel()

	var empty WebhookPayload
	assert.Equal(t, "", webhookPayloadPhoneNumberID(&empty))

	var payload WebhookPayload
	require.NoError(t, json.Unmarshal(signedWebhookBody("phone-xyz"), &payload))
	assert.Equal(t, "phone-xyz", webhookPayloadPhoneNumberID(&payload))
}

// buildSignedWebhookApp creates an App wired with a test DB + Redis for webhook
// signature enforcement tests. Skips when Redis is unavailable.
func buildSignedWebhookApp(t *testing.T, requireSig bool) *App {
	t.Helper()
	redis := testutil.SetupTestRedis(t)
	if redis == nil {
		t.Skip("TEST_REDIS_URL not set, skipping test")
	}
	return &App{
		DB:     testutil.SetupTestDB(t),
		Log:    testutil.NopLogger(),
		Redis:  redis,
		Config: &config.Config{WhatsApp: config.WhatsAppConfig{RequireWebhookSignature: requireSig}},
	}
}

func seedSignedWebhookAccount(t *testing.T, app *App, phoneID, appSecret string) {
	t.Helper()
	org := models.Organization{
		BaseModel: models.BaseModel{ID: uuid.New()},
		Name:      "sig-" + phoneID,
		Slug:      "sig-" + phoneID,
	}
	require.NoError(t, app.DB.Create(&org).Error)

	account := models.WhatsAppAccount{
		BaseModel:          models.BaseModel{ID: uuid.New()},
		OrganizationID:     org.ID,
		Name:               "sig-acct-" + phoneID,
		PhoneID:            phoneID,
		BusinessID:         "biz-" + phoneID,
		AccessToken:        "token",
		AppSecret:          appSecret,
		WebhookVerifyToken: "verify-" + phoneID,
		APIVersion:         "v21.0",
		Status:             "active",
	}
	require.NoError(t, app.DB.Create(&account).Error)
}

func signedWebhookBody(phoneID string) []byte {
	return []byte(fmt.Sprintf(`{"object":"whatsapp_business_account","entry":[{"id":"waba-1","changes":[{"field":"messages","value":{"messaging_product":"whatsapp","metadata":{"display_phone_number":"1555","phone_number_id":%q},"messages":[],"statuses":[]}}]}]}`, phoneID))
}

func hmacSignature(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func callWebhook(t *testing.T, app *App, body []byte, signature string) int {
	t.Helper()
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.Header.SetContentType("application/json")
	if signature != "" {
		ctx.Request.Header.Set("X-Hub-Signature-256", signature)
	}
	ctx.Request.SetBody(body)
	req := &fastglue.Request{RequestCtx: ctx}
	require.NoError(t, app.WebhookHandler(req))
	return ctx.Response.StatusCode()
}

func TestWebhookHandler_AppSecret_ValidSignature_Accepted(t *testing.T) {
	app := buildSignedWebhookApp(t, false)
	const phoneID, secret = "phone-sig-valid", "app-secret-1"
	seedSignedWebhookAccount(t, app, phoneID, secret)

	body := signedWebhookBody(phoneID)
	status := callWebhook(t, app, body, hmacSignature(body, secret))
	assert.Equal(t, fasthttp.StatusOK, status)
}

func TestWebhookHandler_AppSecret_MissingSignature_Rejected(t *testing.T) {
	app := buildSignedWebhookApp(t, false)
	const phoneID, secret = "phone-sig-missing", "app-secret-2"
	seedSignedWebhookAccount(t, app, phoneID, secret)

	status := callWebhook(t, app, signedWebhookBody(phoneID), "")
	assert.Equal(t, fasthttp.StatusForbidden, status)
}

func TestWebhookHandler_AppSecret_InvalidSignature_Rejected(t *testing.T) {
	app := buildSignedWebhookApp(t, false)
	const phoneID, secret = "phone-sig-invalid", "app-secret-3"
	seedSignedWebhookAccount(t, app, phoneID, secret)

	body := signedWebhookBody(phoneID)
	status := callWebhook(t, app, body, hmacSignature(body, "wrong-secret"))
	assert.Equal(t, fasthttp.StatusForbidden, status)
}

func TestWebhookHandler_NoAppSecret_RequireFlag_Rejected(t *testing.T) {
	app := buildSignedWebhookApp(t, true)
	const phoneID = "phone-sig-nosecret-strict"
	seedSignedWebhookAccount(t, app, phoneID, "") // no app secret, no meta config

	status := callWebhook(t, app, signedWebhookBody(phoneID), "")
	assert.Equal(t, fasthttp.StatusForbidden, status)
}

func TestWebhookHandler_NoAppSecret_LenientDefault_Accepted(t *testing.T) {
	app := buildSignedWebhookApp(t, false)
	const phoneID = "phone-sig-nosecret-lenient"
	seedSignedWebhookAccount(t, app, phoneID, "")

	status := callWebhook(t, app, signedWebhookBody(phoneID), "")
	assert.Equal(t, fasthttp.StatusOK, status)
}
