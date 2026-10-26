package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression tests for upstream behaviour changes that are now opt-in
// ChatbotSettings flags defaulting to our main-branch behaviour.

func createOptInChatbotSettings(t *testing.T, app *App, orgID uuid.UUID, mutate func(*models.ChatbotSettings)) {
	t.Helper()
	settings := &models.ChatbotSettings{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: orgID,
		IsEnabled:      true,
	}
	if mutate != nil {
		mutate(settings)
	}
	require.NoError(t, app.DB.Create(settings).Error)
}

func TestSaveIncomingMessage_BotHandledStaysUnreadByDefault(t *testing.T) {
	app := newProcessorTestApp(t)
	org, account := createProcessorTestOrg(t, app)
	contact := testutil.CreateTestContact(t, app.DB, org.ID)
	createOptInChatbotSettings(t, app, org.ID, nil)

	waMsgID := "wamid." + uuid.New().String()[:16]
	app.saveIncomingMessage(account, contact, waMsgID, "text", "hello bot", nil, "")

	var msg models.Message
	require.NoError(t, app.DB.Where("whats_app_message_id = ?", waMsgID).First(&msg).Error)
	assert.Equal(t, models.MessageStatusReceived, msg.Status, "main behaviour: bot-handled messages stay unread")
}

func TestSaveIncomingMessage_BotHandledMarkedReadWhenOptedIn(t *testing.T) {
	app := newProcessorTestApp(t)
	org, account := createProcessorTestOrg(t, app)
	contact := testutil.CreateTestContact(t, app.DB, org.ID)
	createOptInChatbotSettings(t, app, org.ID, func(s *models.ChatbotSettings) { s.MarkBotHandledAsRead = true })

	waMsgID := "wamid." + uuid.New().String()[:16]
	app.saveIncomingMessage(account, contact, waMsgID, "text", "hello bot", nil, "")

	var msg models.Message
	require.NoError(t, app.DB.Where("whats_app_message_id = ?", waMsgID).First(&msg).Error)
	assert.Equal(t, models.MessageStatusRead, msg.Status)
}

func botReplyReadFixture(t *testing.T, markRead bool) (*App, *models.Message, *models.Contact) {
	t.Helper()
	app := newProcessorTestApp(t)
	org, account := createProcessorTestOrg(t, app)
	contact := testutil.CreateTestContact(t, app.DB, org.ID)
	require.NoError(t, app.DB.Model(contact).Update("is_read", false).Error)
	createOptInChatbotSettings(t, app, org.ID, func(s *models.ChatbotSettings) { s.MarkBotHandledAsRead = markRead })

	incoming := &models.Message{
		BaseModel:         models.BaseModel{ID: uuid.New()},
		OrganizationID:    org.ID,
		WhatsAppAccount:   account.Name,
		ContactID:         contact.ID,
		WhatsAppMessageID: "wamid.in_" + uuid.New().String()[:8],
		Direction:         models.DirectionIncoming,
		MessageType:       models.MessageTypeText,
		Content:           "question",
		Status:            models.MessageStatusReceived,
	}
	require.NoError(t, app.DB.Create(incoming).Error)

	require.NoError(t, app.sendAndSaveTextMessage(account, contact, "bot answer"))
	app.wg.Wait()
	return app, incoming, contact
}

func TestChatbotReply_DoesNotMarkIncomingReadByDefault(t *testing.T) {
	app, incoming, contact := botReplyReadFixture(t, false)

	require.NoError(t, app.DB.First(incoming, incoming.ID).Error)
	assert.Equal(t, models.MessageStatusReceived, incoming.Status)
	var dbContact models.Contact
	require.NoError(t, app.DB.First(&dbContact, contact.ID).Error)
	assert.False(t, dbContact.IsRead)
}

func TestChatbotReply_MarksIncomingReadWhenOptedIn(t *testing.T) {
	app, incoming, contact := botReplyReadFixture(t, true)

	require.NoError(t, app.DB.First(incoming, incoming.ID).Error)
	assert.Equal(t, models.MessageStatusRead, incoming.Status)
	var dbContact models.Contact
	require.NoError(t, app.DB.First(&dbContact, contact.ID).Error)
	assert.True(t, dbContact.IsRead)
}

// closedTodayHours returns business hours where today is disabled, so the
// current time is always outside business hours.
func closedTodayHours() models.JSONBArray {
	return models.JSONBArray{map[string]any{
		"day":        float64(time.Now().Weekday()),
		"enabled":    false,
		"start_time": "00:00",
		"end_time":   "23:59",
	}}
}

func runTransferStepOutsideHours(t *testing.T, skip bool) (*App, *models.Contact, *models.ChatbotSession) {
	t.Helper()
	app := newProcessorTestApp(t)
	org, account := createProcessorTestOrg(t, app)
	contact := testutil.CreateTestContactWith(t, app.DB, org.ID, testutil.WithContactAccount(account.Name))
	createOptInChatbotSettings(t, app, org.ID, func(s *models.ChatbotSettings) {
		s.BusinessHours.Enabled = true
		s.BusinessHours.Hours = closedTodayHours()
		s.BusinessHours.OutOfHoursMessage = "We are closed"
		s.BusinessHours.SkipTransferOutsideHours = skip
	})

	flow := &models.ChatbotFlow{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		WhatsAppAccount: account.Name,
		Name:            "Transfer flow",
		IsEnabled:       true,
	}
	require.NoError(t, app.DB.Create(flow).Error)
	session := &models.ChatbotSession{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		ContactID:       contact.ID,
		WhatsAppAccount: account.Name,
		PhoneNumber:     contact.PhoneNumber,
		Status:          models.SessionStatusActive,
		CurrentFlowID:   &flow.ID,
		CurrentStep:     "handoff",
		SessionData:     models.JSONB{},
		StartedAt:       time.Now(),
		LastActivityAt:  time.Now(),
	}
	require.NoError(t, app.DB.Create(session).Error)

	step := &models.ChatbotFlowStep{
		BaseModel:   models.BaseModel{ID: uuid.New()},
		FlowID:      flow.ID,
		StepName:    "handoff",
		MessageType: models.FlowStepTypeTransfer,
		Message:     "Connecting you to an agent",
	}
	app.sendStepMessage(account, session, contact, step)
	app.wg.Wait()
	return app, contact, session
}

func TestTransferStep_OutsideHoursStillTransfersByDefault(t *testing.T) {
	app, contact, _ := runTransferStepOutsideHours(t, false)

	var transfers int64
	require.NoError(t, app.DB.Model(&models.AgentTransfer{}).Where("contact_id = ?", contact.ID).Count(&transfers).Error)
	assert.EqualValues(t, 1, transfers, "main behaviour: transfer step always transfers")
}

func TestTransferStep_OutsideHoursSkipsTransferWhenOptedIn(t *testing.T) {
	app, contact, session := runTransferStepOutsideHours(t, true)

	var transfers int64
	require.NoError(t, app.DB.Model(&models.AgentTransfer{}).Where("contact_id = ?", contact.ID).Count(&transfers).Error)
	assert.EqualValues(t, 0, transfers)

	var outOfHours int64
	require.NoError(t, app.DB.Model(&models.Message{}).
		Where("contact_id = ? AND direction = ? AND content = ?", contact.ID, models.DirectionOutgoing, "We are closed").
		Count(&outOfHours).Error)
	assert.EqualValues(t, 1, outOfHours)

	var dbSession models.ChatbotSession
	require.NoError(t, app.DB.First(&dbSession, session.ID).Error)
	assert.Equal(t, models.SessionStatusCompleted, dbSession.Status)
}

// Finding 5: the sender number is only a fallback for {{phone_number}}; a
// flow-collected phone_number is never overwritten and the default
// completion payload's session_data matches what the flow collected.
func TestSendFlowCompletionWebhook_KeepsFlowCollectedPhoneNumber(t *testing.T) {
	app := newFetchApiResponseTestApp(t)

	var bodies []map[string]any
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		bodies = append(bodies, body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	contact := &models.Contact{BaseModel: models.BaseModel{ID: uuid.New()}, PhoneNumber: "+15550000001"}

	// Flow collected its own phone_number: must be kept everywhere.
	collected := &models.ChatbotSession{
		BaseModel:   models.BaseModel{ID: uuid.New()},
		PhoneNumber: "+15550000001",
		SessionData: models.JSONB{"phone_number": "+15559999999", "name": "Ann"},
	}
	flow := &models.ChatbotFlow{
		BaseModel:        models.BaseModel{ID: uuid.New()},
		Name:             "f",
		CompletionConfig: models.JSONB{"url": server.URL + "/hook/{{phone_number}}"},
	}
	app.sendFlowCompletionWebhook(flow, collected, contact)

	require.Len(t, bodies, 1)
	assert.Equal(t, "/hook/+15559999999", paths[0])
	assert.Equal(t, "+15559999999", collected.SessionData["phone_number"], "session value must not be overwritten")
	sd := bodies[0]["session_data"].(map[string]any)
	assert.Equal(t, "+15559999999", sd["phone_number"])
	assert.Equal(t, "+15550000001", bodies[0]["phone_number"], "top-level phone_number stays the sender, as on main")

	// Flow did not collect phone_number: sender is used for substitution
	// only; the payload's session_data is unchanged from main (no key added).
	plain := &models.ChatbotSession{
		BaseModel:   models.BaseModel{ID: uuid.New()},
		PhoneNumber: "+15550000001",
		SessionData: models.JSONB{"name": "Bob"},
	}
	app.sendFlowCompletionWebhook(flow, plain, contact)

	require.Len(t, bodies, 2)
	assert.Equal(t, "/hook/+15550000001", paths[1])
	_, added := plain.SessionData["phone_number"]
	assert.False(t, added, "session data must not gain a phone_number key")
	sd = bodies[1]["session_data"].(map[string]any)
	assert.Equal(t, map[string]any{"name": "Bob"}, sd)
}

func TestFetchApiResponse_KeepsFlowCollectedPhoneNumber(t *testing.T) {
	app := newFetchApiResponseTestApp(t)

	var capturedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}))
	t.Cleanup(server.Close)

	session := &models.ChatbotSession{
		PhoneNumber: "+15550000001",
		SessionData: models.JSONB{"phone_number": "+15559999999"},
	}
	_, err := app.fetchApiResponse(models.JSONB{"url": server.URL + "/x?p={{phone_number}}", "method": "GET"}, session, "")
	require.NoError(t, err)
	assert.Contains(t, capturedQuery, "15559999999")
	assert.Equal(t, "+15559999999", session.SessionData["phone_number"])

	session2 := &models.ChatbotSession{PhoneNumber: "+15550000001", SessionData: models.JSONB{}}
	_, err = app.fetchApiResponse(models.JSONB{"url": server.URL + "/x?p={{phone_number}}", "method": "GET"}, session2, "")
	require.NoError(t, err)
	assert.Contains(t, capturedQuery, "15550000001")
	_, added := session2.SessionData["phone_number"]
	assert.False(t, added, "sender number must not be persisted into the session")
}
