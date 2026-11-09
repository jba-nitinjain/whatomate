package handlers_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/handlers"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// permCase describes one guarded handler: a caller without any of the grant
// keys must get 403, and a caller holding any single grant key must get past
// the permission check (wantOK = the exact status expected, 0 = anything but
// 401/403, e.g. a 400/404 caused by the empty test body).
type permCase struct {
	name   string
	call   func(app *handlers.App, r *fastglue.Request) error
	grant  []string
	body   any
	params map[string]string
	wantOK int
}

// userWithKeys creates a user whose role grants exactly the given permission
// keys. "tags:read" stands in for "has a role, just not the right one".
func userWithKeys(t *testing.T, app *handlers.App, orgID uuid.UUID, keys ...string) *models.User {
	t.Helper()
	if len(keys) == 0 {
		keys = []string{"tags:read"}
	}
	return testutil.CreateTestUser(t, app.DB, orgID,
		testutil.WithEmail(testutil.UniqueEmail("perm-guard")), withPerms(t, app, orgID, keys...))
}

func callAs(t *testing.T, app *handlers.App, orgID, userID uuid.UUID, c permCase) int {
	t.Helper()
	var req *fastglue.Request
	if c.body != nil {
		req = testutil.NewJSONRequest(t, c.body)
	} else {
		req = testutil.NewGETRequest(t)
	}
	testutil.SetAuthContext(req, orgID, userID)
	for k, v := range c.params {
		req.RequestCtx.SetUserValue(k, v)
	}
	require.NoError(t, c.call(app, req))
	return testutil.GetResponseStatusCode(req)
}

func runPermCases(t *testing.T, cases []permCase) {
	t.Helper()
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	for _, c := range cases {
		t.Run(c.name+"/denied", func(t *testing.T) {
			u := userWithKeys(t, app, org.ID)
			assert.Equal(t, fasthttp.StatusForbidden, callAs(t, app, org.ID, u.ID, c))
		})
		for _, key := range c.grant {
			t.Run(c.name+"/allowed_"+key, func(t *testing.T) {
				u := userWithKeys(t, app, org.ID, key)
				status := callAs(t, app, org.ID, u.ID, c)
				if c.wantOK != 0 {
					assert.Equal(t, c.wantOK, status)
				} else {
					assert.NotEqual(t, fasthttp.StatusForbidden, status)
					assert.NotEqual(t, fasthttp.StatusUnauthorized, status)
				}
			})
		}
	}
}

var missingID = map[string]string{"id": uuid.New().String()}

func TestPermissionGuard_RolesAndSettings(t *testing.T) {
	runPermCases(t, []permCase{
		{name: "ListRoles", call: (*handlers.App).ListRoles,
			grant: []string{"roles:read", "users:read", "organizations:assign"}, wantOK: fasthttp.StatusOK},
		{name: "ListPermissions", call: (*handlers.App).ListPermissions,
			grant: []string{"roles:read"}, wantOK: fasthttp.StatusOK},
		{name: "CreateRole", call: (*handlers.App).CreateRole,
			grant: []string{"roles:write"}, body: map[string]any{"name": "guard-" + uuid.NewString()[:8]}, wantOK: fasthttp.StatusOK},
		{name: "DeleteRole", call: (*handlers.App).DeleteRole,
			grant: []string{"roles:delete"}, params: missingID, wantOK: fasthttp.StatusNotFound},
		{name: "GetSSOSettings", call: (*handlers.App).GetSSOSettings,
			grant: []string{"settings.sso:read"}, wantOK: fasthttp.StatusOK},
		{name: "DeleteSSOProvider", call: (*handlers.App).DeleteSSOProvider,
			grant: []string{"settings.sso:write"}, params: map[string]string{"provider": "google"}},
		{name: "GetOrganizationSettings", call: (*handlers.App).GetOrganizationSettings,
			grant: []string{"settings.general:read"}, wantOK: fasthttp.StatusOK},
		{name: "UpdateOrganizationSettings", call: (*handlers.App).UpdateOrganizationSettings,
			grant: []string{"settings.general:write"}, body: map[string]any{"timezone": "Asia/Kolkata"}, wantOK: fasthttp.StatusOK},
		{name: "GetDashboardStats", call: (*handlers.App).GetDashboardStats,
			grant: []string{"analytics:read"}, wantOK: fasthttp.StatusOK},
		{name: "GetAllWidgetsData", call: (*handlers.App).GetAllWidgetsData,
			grant: []string{"analytics:read"}, wantOK: fasthttp.StatusOK},
		{name: "SaveWidgetLayout", call: (*handlers.App).SaveWidgetLayout,
			grant: []string{"analytics:write"}, body: map[string]any{"layout": []any{}}, wantOK: fasthttp.StatusBadRequest},
	})
}

func TestPermissionGuard_Messaging(t *testing.T) {
	runPermCases(t, []permCase{
		// Agents hold chat:read but not templates:read; the chat template picker must keep working.
		{name: "ListTemplates", call: (*handlers.App).ListTemplates,
			grant: []string{"templates:read", "chat:read"}, wantOK: fasthttp.StatusOK},
		{name: "CreateTemplate", call: (*handlers.App).CreateTemplate,
			grant: []string{"templates:write"}, body: map[string]any{}},
		{name: "DeleteTemplate", call: (*handlers.App).DeleteTemplate,
			grant: []string{"templates:delete"}, params: missingID, wantOK: fasthttp.StatusNotFound},
		{name: "SyncTemplates", call: (*handlers.App).SyncTemplates,
			grant: []string{"templates:sync"}, body: map[string]any{}},
		{name: "ListCannedResponses", call: (*handlers.App).ListCannedResponses,
			grant: []string{"canned_responses:read"}, wantOK: fasthttp.StatusOK},
		{name: "CreateCannedResponse", call: (*handlers.App).CreateCannedResponse,
			grant: []string{"canned_responses:write"}, body: map[string]any{}},
		{name: "DeleteCannedResponse", call: (*handlers.App).DeleteCannedResponse,
			grant: []string{"canned_responses:delete"}, params: missingID, wantOK: fasthttp.StatusNotFound},
		// Agents run custom actions from the chat screen.
		{name: "ListCustomActions", call: (*handlers.App).ListCustomActions,
			grant: []string{"custom_actions:read", "chat:read"}, wantOK: fasthttp.StatusOK},
		{name: "CreateCustomAction", call: (*handlers.App).CreateCustomAction,
			grant: []string{"custom_actions:write"}, body: map[string]any{}},
		{name: "ExecuteCustomAction", call: (*handlers.App).ExecuteCustomAction,
			grant: []string{"chat:write", "custom_actions:write"}, params: missingID, body: map[string]any{"contact_id": uuid.NewString()}},
		{name: "ListWebhooks", call: (*handlers.App).ListWebhooks,
			grant: []string{"webhooks:read"}, wantOK: fasthttp.StatusOK},
		{name: "DeleteWebhook", call: (*handlers.App).DeleteWebhook,
			grant: []string{"webhooks:delete"}, params: missingID, wantOK: fasthttp.StatusNotFound},
		{name: "CreateAgentTransfer", call: (*handlers.App).CreateAgentTransfer,
			grant: []string{"transfers:write"}, body: map[string]any{}},
	})
}

func TestPermissionGuard_CampaignsAndRSVP(t *testing.T) {
	runPermCases(t, []permCase{
		{name: "ListCampaigns", call: (*handlers.App).ListCampaigns,
			grant: []string{"campaigns:read"}, wantOK: fasthttp.StatusOK},
		{name: "CreateCampaign", call: (*handlers.App).CreateCampaign,
			grant: []string{"campaigns:write"}, body: map[string]any{}},
		{name: "DeleteCampaign", call: (*handlers.App).DeleteCampaign,
			grant: []string{"campaigns:delete"}, params: missingID, wantOK: fasthttp.StatusNotFound},
		{name: "StartCampaign", call: (*handlers.App).StartCampaign,
			grant: []string{"campaigns:execute"}, params: missingID, wantOK: fasthttp.StatusNotFound},
		{name: "ListRSVPEvents", call: (*handlers.App).ListRSVPEvents,
			grant: []string{"rsvp:read"}, wantOK: fasthttp.StatusOK},
		{name: "CreateRSVPEvent", call: (*handlers.App).CreateRSVPEvent,
			grant: []string{"rsvp:write"}, body: map[string]any{}},
		{name: "DeleteRSVPEvent", call: (*handlers.App).DeleteRSVPEvent,
			grant: []string{"rsvp:delete"}, params: missingID},
		{name: "ExportRSVPResponses", call: (*handlers.App).ExportRSVPResponses,
			grant: []string{"rsvp:export"}, params: missingID},
		{name: "RepromptRSVPFlowSessions", call: (*handlers.App).RepromptRSVPFlowSessions,
			grant: []string{"rsvp:execute"}, params: missingID, body: map[string]any{}},
	})
}

func TestPermissionGuard_ChatbotFlowsAndAccounts(t *testing.T) {
	runPermCases(t, []permCase{
		// Agents (transfers:read) read allow_agent_queue_pickup from the chatbot settings.
		{name: "GetChatbotSettings", call: (*handlers.App).GetChatbotSettings,
			grant: []string{"settings.chatbot:read", "transfers:read"}, wantOK: fasthttp.StatusOK},
		{name: "UpdateChatbotSettings", call: (*handlers.App).UpdateChatbotSettings,
			grant: []string{"settings.chatbot:write"}, body: map[string]any{}},
		{name: "ListKeywordRules", call: (*handlers.App).ListKeywordRules,
			grant: []string{"chatbot.keywords:read"}, wantOK: fasthttp.StatusOK},
		{name: "DeleteKeywordRule", call: (*handlers.App).DeleteKeywordRule,
			grant: []string{"chatbot.keywords:delete"}, params: missingID},
		{name: "ListAIContexts", call: (*handlers.App).ListAIContexts,
			grant: []string{"chatbot.ai:read"}, wantOK: fasthttp.StatusOK},
		{name: "CreateAIContext", call: (*handlers.App).CreateAIContext,
			grant: []string{"chatbot.ai:write"}, body: map[string]any{}},
		{name: "ListFlows", call: (*handlers.App).ListFlows,
			grant: []string{"flows.whatsapp:read", "flows.chatbot:read"}, wantOK: fasthttp.StatusOK},
		{name: "DeleteFlow", call: (*handlers.App).DeleteFlow,
			grant: []string{"flows.whatsapp:delete"}, params: missingID, wantOK: fasthttp.StatusNotFound},
		{name: "UploadFlowMedia", call: (*handlers.App).UploadFlowMedia,
			grant: []string{"flows.chatbot:write"}, body: map[string]any{}},
		{name: "TestAccountConnection", call: (*handlers.App).TestAccountConnection,
			grant: []string{"accounts:read"}, params: missingID, wantOK: fasthttp.StatusNotFound},
		{name: "SubscribeApp", call: (*handlers.App).SubscribeApp,
			grant: []string{"accounts:write"}, params: missingID},
		{name: "UpdateBusinessProfile", call: (*handlers.App).UpdateBusinessProfile,
			grant: []string{"accounts:write"}, params: missingID, body: map[string]any{}},
		{name: "RequestAccountPhoneVerificationCode", call: (*handlers.App).RequestAccountPhoneVerificationCode,
			grant: []string{"accounts:write"}, params: missingID, body: map[string]any{}},
		{name: "GetWhatsAppOnboardingSession", call: (*handlers.App).GetWhatsAppOnboardingSession,
			grant: []string{"accounts:read"}, params: missingID},
		{name: "FinalizeWhatsAppOnboarding", call: (*handlers.App).FinalizeWhatsAppOnboarding,
			grant: []string{"accounts:write"}, params: missingID, body: map[string]any{}},
	})
}

// GET /api/users/{id}: anyone may read their own record, others need users:read.
func TestPermissionGuard_GetUser(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	other := userWithKeys(t, app, org.ID)
	plain := userWithKeys(t, app, org.ID)
	reader := userWithKeys(t, app, org.ID, "users:read")

	get := func(caller, target uuid.UUID) int {
		req := testutil.NewGETRequest(t)
		testutil.SetAuthContext(req, org.ID, caller)
		req.RequestCtx.SetUserValue("id", target.String())
		require.NoError(t, app.GetUser(req))
		return testutil.GetResponseStatusCode(req)
	}

	assert.Equal(t, fasthttp.StatusOK, get(plain.ID, plain.ID), "own record")
	assert.Equal(t, fasthttp.StatusForbidden, get(plain.ID, other.ID), "other user without users:read")
	assert.Equal(t, fasthttp.StatusOK, get(reader.ID, other.ID), "other user with users:read")
}

// Meta app credentials keep needing accounts:write only; every other field
// needs settings.general:write.
func TestPermissionGuard_UpdateOrganizationSettings_MetaFieldsOnly(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	accountsWriter := userWithKeys(t, app, org.ID, "accounts:write")

	update := func(body map[string]any) int {
		req := testutil.NewJSONRequest(t, body)
		testutil.SetAuthContext(req, org.ID, accountsWriter.ID)
		require.NoError(t, app.UpdateOrganizationSettings(req))
		return testutil.GetResponseStatusCode(req)
	}

	assert.Equal(t, fasthttp.StatusOK, update(map[string]any{"meta_app_id": "1234567890"}))
	assert.Equal(t, fasthttp.StatusForbidden, update(map[string]any{"meta_app_id": "1234567890", "timezone": "UTC"}))
}

// Super admins hold every permission even without a role.
func TestPermissionGuard_SuperAdminBypass(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	sa := testutil.CreateTestUser(t, app.DB, org.ID,
		testutil.WithEmail(testutil.UniqueEmail("perm-guard-sa")), testutil.WithSuperAdmin())

	for name, call := range map[string]func(*fastglue.Request) error{
		"ListRoles":      app.ListRoles,
		"GetSSOSettings": app.GetSSOSettings,
		"ListCampaigns":  app.ListCampaigns,
		"ListWebhooks":   app.ListWebhooks,
	} {
		req := testutil.NewGETRequest(t)
		testutil.SetAuthContext(req, org.ID, sa.ID)
		require.NoError(t, call(req))
		assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req), name)
	}
}
