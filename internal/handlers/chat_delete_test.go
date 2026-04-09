package handlers_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func TestApp_DeleteMessage(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&adminRole.ID))
	account := testutil.CreateTestWhatsAppAccount(t, app.DB, org.ID)
	contact := testutil.CreateTestContactWith(t, app.DB, org.ID, testutil.WithContactAccount(account.Name))

	older := &models.Message{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		WhatsAppAccount: account.Name,
		ContactID:       contact.ID,
		Direction:       models.DirectionIncoming,
		MessageType:     models.MessageTypeText,
		Content:         "Older message",
		Status:          models.MessageStatusRead,
	}
	latest := &models.Message{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		WhatsAppAccount: account.Name,
		ContactID:       contact.ID,
		Direction:       models.DirectionOutgoing,
		MessageType:     models.MessageTypeText,
		Content:         "Latest message",
		Status:          models.MessageStatusSent,
		IsReply:         true,
		ReplyToMessageID: &older.ID,
	}
	reply := &models.Message{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		WhatsAppAccount: account.Name,
		ContactID:       contact.ID,
		Direction:       models.DirectionOutgoing,
		MessageType:     models.MessageTypeText,
		Content:         "Replying to latest",
		Status:          models.MessageStatusSent,
		IsReply:         true,
		ReplyToMessageID: &latest.ID,
	}
	require.NoError(t, app.DB.Create(older).Error)
	require.NoError(t, app.DB.Create(latest).Error)
	require.NoError(t, app.DB.Create(reply).Error)
	require.NoError(t, app.DB.Model(&contact).Updates(map[string]any{
		"last_message_at":      reply.CreatedAt,
		"last_message_preview": reply.Content,
		"last_inbound_at":      older.CreatedAt,
	}).Error)

	req := testutil.NewJSONRequest(t, map[string]any{})
	req.RequestCtx.Request.Header.SetMethod(fasthttp.MethodDelete)
	testutil.SetAuthContext(req, org.ID, user.ID)
	testutil.SetPathParam(req, "id", contact.ID.String())
	testutil.SetPathParam(req, "message_id", latest.ID.String())

	err := app.DeleteMessage(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp struct {
		Data struct {
			MessageID string                   `json:"message_id"`
			Contact   handlers.ContactResponse `json:"contact"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(testutil.GetResponseBody(req), &resp))
	assert.Equal(t, latest.ID.String(), resp.Data.MessageID)
	assert.Equal(t, reply.Content, resp.Data.Contact.LastMessagePreview)

	var deleted models.Message
	require.NoError(t, app.DB.Unscoped().Where("id = ?", latest.ID).First(&deleted).Error)
	assert.True(t, deleted.DeletedAt.Valid)

	var updatedReply models.Message
	require.NoError(t, app.DB.Where("id = ?", reply.ID).First(&updatedReply).Error)
	assert.False(t, updatedReply.IsReply)
	assert.Nil(t, updatedReply.ReplyToMessageID)
}

func TestApp_DeleteContact_WithConversation(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&adminRole.ID))
	account := testutil.CreateTestWhatsAppAccount(t, app.DB, org.ID)
	contact := testutil.CreateTestContactWith(t, app.DB, org.ID, testutil.WithContactAccount(account.Name))

	msg := &models.Message{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		WhatsAppAccount: account.Name,
		ContactID:       contact.ID,
		Direction:       models.DirectionIncoming,
		MessageType:     models.MessageTypeText,
		Content:         "Delete me",
		Status:          models.MessageStatusDelivered,
	}
	require.NoError(t, app.DB.Create(msg).Error)

	req := testutil.NewJSONRequest(t, map[string]any{})
	req.RequestCtx.Request.Header.SetMethod(fasthttp.MethodDelete)
	req.RequestCtx.Request.URI().QueryArgs().Set("include_messages", "true")
	testutil.SetAuthContext(req, org.ID, user.ID)
	testutil.SetPathParam(req, "id", contact.ID.String())

	err := app.DeleteContact(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var deletedContact models.Contact
	require.NoError(t, app.DB.Unscoped().Where("id = ?", contact.ID).First(&deletedContact).Error)
	assert.True(t, deletedContact.DeletedAt.Valid)

	var deletedMessage models.Message
	require.NoError(t, app.DB.Unscoped().Where("id = ?", msg.ID).First(&deletedMessage).Error)
	assert.True(t, deletedMessage.DeletedAt.Valid)
}
