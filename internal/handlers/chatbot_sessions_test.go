package handlers_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/handlers"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

type sessionListResp struct {
	Data struct {
		Sessions []handlers.ChatbotSessionResponse `json:"sessions"`
		Total    int64                             `json:"total"`
		Page     int                               `json:"page"`
		Limit    int                               `json:"limit"`
	} `json:"data"`
}

func listSessions(t *testing.T, app *handlers.App, orgID, userID uuid.UUID, params map[string]string) (int, sessionListResp) {
	t.Helper()
	req := testutil.NewGETRequest(t)
	testutil.SetAuthContext(req, orgID, userID)
	for k, v := range params {
		testutil.SetQueryParam(req, k, v)
	}
	require.NoError(t, app.ListChatbotSessions(req))
	var resp sessionListResp
	if testutil.GetResponseStatusCode(req) == fasthttp.StatusOK {
		require.NoError(t, json.Unmarshal(testutil.GetResponseBody(req), &resp))
	}
	return testutil.GetResponseStatusCode(req), resp
}

func TestApp_ChatbotSessions_RequiresFlowsChatbotRead(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	noPerm := testutil.CreateTestUser(t, app.DB, org.ID)

	status, _ := listSessions(t, app, org.ID, noPerm.ID, nil)
	assert.Equal(t, fasthttp.StatusForbidden, status)

	req := testutil.NewGETRequest(t)
	testutil.SetAuthContext(req, org.ID, noPerm.ID)
	testutil.SetPathParam(req, "id", uuid.New().String())
	require.NoError(t, app.GetChatbotSession(req))
	assert.Equal(t, fasthttp.StatusForbidden, testutil.GetResponseStatusCode(req))
}

func TestApp_ListChatbotSessions_FiltersAndPagination(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	user := testutil.CreateTestUser(t, app.DB, org.ID, sessionReaderRole(t, app, org.ID))

	alice := testutil.CreateTestContactWith(t, app.DB, org.ID, testutil.WithPhoneNumber("+919800000001"))
	require.NoError(t, app.DB.Model(alice).Update("profile_name", "Alice Sharma").Error)
	bob := testutil.CreateTestContactWith(t, app.DB, org.ID, testutil.WithPhoneNumber("+919800000002"))

	flow := &models.ChatbotFlow{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		WhatsAppAccount: "test-account",
		Name:            "Lead Capture",
	}
	require.NoError(t, app.DB.Create(flow).Error)

	s1 := createSessionForChatbotTest(t, app, org.ID, alice.ID, alice.PhoneNumber, models.SessionStatusCompleted)
	require.NoError(t, app.DB.Model(s1).Update("current_flow_id", flow.ID).Error)
	require.NoError(t, app.DB.Create(&models.ChatbotSessionMessage{
		BaseModel: models.BaseModel{ID: uuid.New()}, SessionID: s1.ID,
		Direction: models.DirectionIncoming, Message: "hi", StepName: "keyword_check",
	}).Error)
	s2 := createSessionForChatbotTest(t, app, org.ID, bob.ID, bob.PhoneNumber, models.SessionStatusActive)
	old := time.Now().AddDate(0, 0, -10)
	require.NoError(t, app.DB.Model(s2).Update("started_at", old).Error)

	t.Run("pagination envelope", func(t *testing.T) {
		status, resp := listSessions(t, app, org.ID, user.ID, map[string]string{"page": "1", "limit": "1"})
		require.Equal(t, fasthttp.StatusOK, status)
		assert.Equal(t, int64(2), resp.Data.Total)
		assert.Len(t, resp.Data.Sessions, 1)
		assert.Equal(t, 1, resp.Data.Limit)
	})

	t.Run("flow filter returns flow name and message count", func(t *testing.T) {
		_, resp := listSessions(t, app, org.ID, user.ID, map[string]string{"flow_id": flow.ID.String()})
		require.Len(t, resp.Data.Sessions, 1)
		assert.Equal(t, s1.ID, resp.Data.Sessions[0].ID)
		assert.Equal(t, "Lead Capture", resp.Data.Sessions[0].FlowName)
		assert.Equal(t, int64(1), resp.Data.Sessions[0].MessageCount)
	})

	t.Run("invalid flow id is rejected", func(t *testing.T) {
		status, _ := listSessions(t, app, org.ID, user.ID, map[string]string{"flow_id": "nope"})
		assert.Equal(t, fasthttp.StatusBadRequest, status)
	})

	t.Run("search by contact name and phone", func(t *testing.T) {
		_, resp := listSessions(t, app, org.ID, user.ID, map[string]string{"search": "alice"})
		require.Len(t, resp.Data.Sessions, 1)
		assert.Equal(t, s1.ID, resp.Data.Sessions[0].ID)

		_, resp = listSessions(t, app, org.ID, user.ID, map[string]string{"search": "0002"})
		require.Len(t, resp.Data.Sessions, 1)
		assert.Equal(t, s2.ID, resp.Data.Sessions[0].ID)
	})

	t.Run("date range on started_at", func(t *testing.T) {
		today := time.Now().UTC().Format("2006-01-02")
		_, resp := listSessions(t, app, org.ID, user.ID, map[string]string{"from": today, "to": today})
		require.Len(t, resp.Data.Sessions, 1)
		assert.Equal(t, s1.ID, resp.Data.Sessions[0].ID)
	})
}

func TestApp_GetChatbotSession_FlowNameFallbackAndOrderedMessages(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	user := testutil.CreateTestUser(t, app.DB, org.ID, sessionReaderRole(t, app, org.ID))
	contact := testutil.CreateTestContact(t, app.DB, org.ID)
	session := createSessionForChatbotTest(t, app, org.ID, contact.ID, "+15550001111", models.SessionStatusActive)
	require.NoError(t, app.DB.Model(session).Update("session_data", models.JSONB{"_flow_name": "Old Flow", "city": "Pune"}).Error)

	base := time.Now().Add(-time.Hour)
	for i, text := range []string{"first", "second", "third"} {
		m := &models.ChatbotSessionMessage{
			BaseModel: models.BaseModel{ID: uuid.New()}, SessionID: session.ID,
			Direction: models.DirectionIncoming, Message: text,
		}
		require.NoError(t, app.DB.Create(m).Error)
		require.NoError(t, app.DB.Model(m).Update("created_at", base.Add(time.Duration(2-i)*-time.Minute)).Error)
	}

	req := testutil.NewGETRequest(t)
	testutil.SetAuthContext(req, org.ID, user.ID)
	testutil.SetPathParam(req, "id", session.ID.String())
	require.NoError(t, app.GetChatbotSession(req))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp struct {
		Data handlers.ChatbotSessionResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(testutil.GetResponseBody(req), &resp))
	assert.Equal(t, "Old Flow", resp.Data.FlowName)
	assert.Equal(t, int64(3), resp.Data.MessageCount)
	assert.Equal(t, "Pune", resp.Data.SessionData["city"])
	require.Len(t, resp.Data.Messages, 3)
	assert.Equal(t, "first", resp.Data.Messages[0].Message)
	assert.Equal(t, "third", resp.Data.Messages[2].Message)
}
