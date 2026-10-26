package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/handlers"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

type sendTemplateFixture struct {
	app     *handlers.App
	mock    *mockWhatsAppServer
	org     *models.Organization
	user    *models.User
	account *models.WhatsAppAccount
	contact *models.Contact
}

func newSendTemplateFixture(t *testing.T) *sendTemplateFixture {
	t.Helper()
	mock := newMockWhatsAppServer()
	t.Cleanup(mock.close)
	app := newMsgTestApp(t, mock)
	org := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&adminRole.ID))
	account := createTestAccount(t, app, org.ID)
	contact := testutil.CreateTestContactWith(t, app.DB, org.ID, testutil.WithContactAccount(account.Name))
	return &sendTemplateFixture{app: app, mock: mock, org: org, user: user, account: account, contact: contact}
}

func (f *sendTemplateFixture) template(t *testing.T, mutate func(*models.Template)) *models.Template {
	t.Helper()
	tpl := &models.Template{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  f.org.ID,
		WhatsAppAccount: f.account.Name,
		Name:            "tpl_" + uuid.New().String()[:8],
		Language:        "en",
		Category:        "UTILITY",
		Status:          string(models.TemplateStatusApproved),
	}
	mutate(tpl)
	require.NoError(t, f.app.DB.Create(tpl).Error)
	return tpl
}

// send posts the request and returns the template components Meta received.
func (f *sendTemplateFixture) send(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	body["contact_id"] = f.contact.ID.String()
	req := testutil.NewJSONRequest(t, body)
	testutil.SetAuthContext(req, f.org.ID, f.user.ID)
	require.NoError(t, f.app.SendTemplateMessage(req))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req), string(testutil.GetResponseBody(req)))
	f.app.WaitForBackgroundTasks()
	require.Len(t, f.mock.sentMessages, 1)
	tpl := f.mock.sentMessages[0]["template"].(map[string]any)
	raw, _ := tpl["components"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, c := range raw {
		out = append(out, c.(map[string]any))
	}
	return out
}

func componentsOfType(components []map[string]any, typ string) []map[string]any {
	var out []map[string]any
	for _, c := range components {
		if c["type"] == typ {
			out = append(out, c)
		}
	}
	return out
}

func firstParam(t *testing.T, comp map[string]any) map[string]any {
	t.Helper()
	params := comp["parameters"].([]any)
	require.NotEmpty(t, params)
	return params[0].(map[string]any)
}

// A request that was valid on main (single positional TEXT header variable
// supplied via template_params) must still succeed.
func TestSendTemplate_PositionalHeaderViaTemplateParamsStillSucceeds(t *testing.T) {
	t.Parallel()
	f := newSendTemplateFixture(t)
	tpl := f.template(t, func(tp *models.Template) {
		tp.HeaderType = "TEXT"
		tp.HeaderContent = "Order {{1}}"
		tp.BodyContent = "Your order {{1}} has shipped."
	})

	components := f.send(t, map[string]any{
		"template_name":   tpl.Name,
		"template_params": map[string]string{"1": "ORD-9"},
	})

	headers := componentsOfType(components, "header")
	require.Len(t, headers, 1)
	assert.Equal(t, "ORD-9", firstParam(t, headers[0])["text"])
}

// Finding 9: button_params values reach Meta for URL buttons, even when the
// value is not in template_params.
func TestSendTemplate_ButtonParamsFillDynamicURLButton(t *testing.T) {
	t.Parallel()
	f := newSendTemplateFixture(t)
	tpl := f.template(t, func(tp *models.Template) {
		tp.BodyContent = "Track your order."
		tp.Buttons = models.JSONBArray{
			map[string]any{"type": "URL", "text": "Track", "url": "https://example.com/t/{{1}}"},
		}
	})

	components := f.send(t, map[string]any{
		"template_name": tpl.Name,
		"button_params": map[string]string{"0": "abc123"},
	})

	buttons := componentsOfType(components, "button")
	require.Len(t, buttons, 1, "URL button must be sent exactly once")
	assert.Equal(t, "url", buttons[0]["sub_type"])
	assert.Equal(t, "0", buttons[0]["index"])
	assert.Equal(t, "abc123", firstParam(t, buttons[0])["text"])
}

// Main behaviour preserved: a URL button value from template_params is sent
// once, and an explicit button_params value for the same index wins.
func TestSendTemplate_ButtonParamsDoNotDuplicateTemplateParamButton(t *testing.T) {
	t.Parallel()
	f := newSendTemplateFixture(t)
	tpl := f.template(t, func(tp *models.Template) {
		tp.BodyContent = "Track your order."
		tp.Buttons = models.JSONBArray{
			map[string]any{"type": "URL", "text": "Track", "url": "https://example.com/t/{{1}}"},
		}
	})

	components := f.send(t, map[string]any{
		"template_name":   tpl.Name,
		"template_params": map[string]string{"1": "from-template-params"},
		"button_params":   map[string]string{"0": "from-button-params"},
	})

	buttons := componentsOfType(components, "button")
	require.Len(t, buttons, 1)
	assert.Equal(t, "from-button-params", firstParam(t, buttons[0])["text"])
}

// Finding 9: upstream OTP auto-fill — AUTHENTICATION templates with an OTP
// button get the code from template_params["1"].
func TestSendTemplate_AuthenticationOTPAutoFill(t *testing.T) {
	t.Parallel()
	f := newSendTemplateFixture(t)
	tpl := f.template(t, func(tp *models.Template) {
		tp.Category = "AUTHENTICATION"
		tp.BodyContent = "{{1}} is your verification code."
		tp.Buttons = models.JSONBArray{
			map[string]any{"type": "OTP", "otp_type": "COPY_CODE", "text": "Copy code"},
		}
	})

	components := f.send(t, map[string]any{
		"template_name":   tpl.Name,
		"template_params": map[string]string{"1": "482913"},
	})

	buttons := componentsOfType(components, "button")
	require.Len(t, buttons, 1)
	assert.Equal(t, "url", buttons[0]["sub_type"])
	assert.Equal(t, "0", buttons[0]["index"])
	assert.Equal(t, "482913", firstParam(t, buttons[0])["text"])
}

// Finding 9: the resolved DOCUMENT filename (here the header_media_url
// basename) is passed to Meta.
func TestSendTemplate_DocumentHeaderFilenameFromURL(t *testing.T) {
	t.Parallel()
	f := newSendTemplateFixture(t)
	files := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4 test"))
	}))
	t.Cleanup(files.Close)

	tpl := f.template(t, func(tp *models.Template) {
		tp.HeaderType = "DOCUMENT"
		tp.BodyContent = "Your invoice is attached."
	})

	components := f.send(t, map[string]any{
		"template_name":    tpl.Name,
		"header_media_url": files.URL + "/files/invoice-42.pdf",
	})

	headers := componentsOfType(components, "header")
	require.Len(t, headers, 1)
	doc := firstParam(t, headers[0])["document"].(map[string]any)
	assert.Equal(t, f.mock.nextMediaID, doc["id"])
	assert.Equal(t, "invoice-42.pdf", doc["filename"])
}
