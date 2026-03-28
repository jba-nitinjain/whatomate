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

func TestApp_CreateExternalMessage(t *testing.T) {
	t.Parallel()

	t.Run("success with existing contact", func(t *testing.T) {
		t.Parallel()

		mockServer := newMockWhatsAppServer()
		defer mockServer.close()

		app := newMsgTestApp(t, mockServer)
		org := testutil.CreateTestOrganization(t, app.DB)
		adminRole := testutil.CreateAdminRole(t, app.DB, org.ID)
		user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&adminRole.ID))
		account := createTestAccount(t, app, org.ID)
		contact := testutil.CreateTestContactWith(t, app.DB, org.ID, testutil.WithContactAccount(account.Name))

		req := testutil.NewJSONRequest(t, map[string]any{
			"contact_id":          contact.ID.String(),
			"whatsapp_account":    account.Name,
			"type":                "text",
			"content":             map[string]string{"body": "Imported from CRM"},
			"external_message_id": "crm-msg-123",
			"whatsapp_message_id": "wamid.external-123",
		})
		testutil.SetAuthContext(req, org.ID, user.ID)

		err := app.CreateExternalMessage(req)
		require.NoError(t, err)
		assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

		var resp struct {
			Data handlers.MessageResponse `json:"data"`
		}
		require.NoError(t, json.Unmarshal(testutil.GetResponseBody(req), &resp))
		assert.Equal(t, contact.ID, resp.Data.ContactID)
		assert.Equal(t, models.DirectionOutgoing, resp.Data.Direction)
		assert.Equal(t, models.MessageTypeText, resp.Data.MessageType)
		assert.Equal(t, models.MessageStatusSent, resp.Data.Status)
		assert.Equal(t, "wamid.external-123", resp.Data.WAMID)

		var msg models.Message
		require.NoError(t, app.DB.First(&msg, resp.Data.ID).Error)
		assert.Equal(t, models.MessageStatusSent, msg.Status)
		assert.Equal(t, "Imported from CRM", msg.Content)
		assert.Equal(t, "wamid.external-123", msg.WhatsAppMessageID)
		assert.Equal(t, "crm-msg-123", msg.Metadata["external_message_id"])
		assert.Equal(t, "external_api", msg.Metadata["source"])
		require.NotNil(t, msg.SentByUserID)
		assert.Equal(t, user.ID, *msg.SentByUserID)

		var updatedContact models.Contact
		require.NoError(t, app.DB.First(&updatedContact, contact.ID).Error)
		assert.Equal(t, "Imported from CRM", updatedContact.LastMessagePreview)
		assert.Equal(t, account.Name, updatedContact.WhatsAppAccount)
		assert.Len(t, mockServer.sentMessages, 0)
	})

	t.Run("success with phone number creates contact", func(t *testing.T) {
		t.Parallel()

		mockServer := newMockWhatsAppServer()
		defer mockServer.close()

		app := newMsgTestApp(t, mockServer)
		org := testutil.CreateTestOrganization(t, app.DB)
		adminRole := testutil.CreateAdminRole(t, app.DB, org.ID)
		user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&adminRole.ID))
		account := createTestAccount(t, app, org.ID)

		req := testutil.NewJSONRequest(t, map[string]any{
			"phone_number":       "+919876543210",
			"profile_name":       "External Contact",
			"whatsapp_account":   account.Name,
			"type":               "text",
			"content":            map[string]string{"body": "Created via external API"},
			"whatsapp_message_id": "wamid.external-456",
		})
		testutil.SetAuthContext(req, org.ID, user.ID)

		err := app.CreateExternalMessage(req)
		require.NoError(t, err)
		assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

		var resp struct {
			Data handlers.MessageResponse `json:"data"`
		}
		require.NoError(t, json.Unmarshal(testutil.GetResponseBody(req), &resp))
		assert.Equal(t, models.MessageStatusSent, resp.Data.Status)

		var contact models.Contact
		require.NoError(t, app.DB.Where("organization_id = ? AND phone_number = ?", org.ID, "919876543210").First(&contact).Error)
		assert.Equal(t, "External Contact", contact.ProfileName)
		assert.Equal(t, account.Name, contact.WhatsAppAccount)

		var msg models.Message
		require.NoError(t, app.DB.Where("contact_id = ?", contact.ID).First(&msg).Error)
		assert.Equal(t, contact.ID, msg.ContactID)
		assert.Equal(t, "Created via external API", msg.Content)
		assert.Equal(t, "wamid.external-456", msg.WhatsAppMessageID)
		assert.Len(t, mockServer.sentMessages, 0)
	})

	t.Run("phone number create requires contacts write", func(t *testing.T) {
		t.Parallel()

		app := newTestApp(t)
		org := testutil.CreateTestOrganization(t, app.DB)
		agentRole := testutil.CreateAgentRole(t, app.DB, org.ID)
		user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&agentRole.ID))

		req := testutil.NewJSONRequest(t, map[string]any{
			"phone_number": "919876543210",
			"type":         "text",
			"content":      map[string]string{"body": "Should fail"},
		})
		testutil.SetAuthContext(req, org.ID, user.ID)

		err := app.CreateExternalMessage(req)
		require.NoError(t, err)
		testutil.AssertErrorResponse(t, req, fasthttp.StatusForbidden, "create contacts")
	})

	t.Run("missing contact identifier", func(t *testing.T) {
		t.Parallel()

		app := newTestApp(t)
		org := testutil.CreateTestOrganization(t, app.DB)
		adminRole := testutil.CreateAdminRole(t, app.DB, org.ID)
		user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&adminRole.ID))

		req := testutil.NewJSONRequest(t, map[string]any{
			"type":    "text",
			"content": map[string]string{"body": "No contact"},
		})
		testutil.SetAuthContext(req, org.ID, user.ID)

		err := app.CreateExternalMessage(req)
		require.NoError(t, err)
		testutil.AssertErrorResponse(t, req, fasthttp.StatusBadRequest, "Either contact_id or phone_number is required")
	})

	t.Run("reply to message must belong to same contact", func(t *testing.T) {
		t.Parallel()

		app := newTestApp(t)
		org := testutil.CreateTestOrganization(t, app.DB)
		adminRole := testutil.CreateAdminRole(t, app.DB, org.ID)
		user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&adminRole.ID))
		account := testutil.CreateTestWhatsAppAccount(t, app.DB, org.ID)
		contact := testutil.CreateTestContactWith(t, app.DB, org.ID, testutil.WithContactAccount(account.Name))
		otherContact := testutil.CreateTestContactWith(t, app.DB, org.ID, testutil.WithContactAccount(account.Name))

		otherMsg := &models.Message{
			BaseModel:         models.BaseModel{ID: uuid.New()},
			OrganizationID:    org.ID,
			WhatsAppAccount:   account.Name,
			ContactID:         otherContact.ID,
			WhatsAppMessageID: "wamid.other",
			Direction:         models.DirectionIncoming,
			MessageType:       models.MessageTypeText,
			Content:           "Other contact message",
			Status:            models.MessageStatusReceived,
		}
		require.NoError(t, app.DB.Create(otherMsg).Error)

		req := testutil.NewJSONRequest(t, map[string]any{
			"contact_id":          contact.ID.String(),
			"whatsapp_account":    account.Name,
			"type":                "text",
			"content":             map[string]string{"body": "Reply should fail"},
			"reply_to_message_id": otherMsg.ID.String(),
		})
		testutil.SetAuthContext(req, org.ID, user.ID)

		err := app.CreateExternalMessage(req)
		require.NoError(t, err)
		testutil.AssertErrorResponse(t, req, fasthttp.StatusNotFound, "Reply-to message not found")
	})
}
