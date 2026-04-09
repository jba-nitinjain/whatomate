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

	t.Run("super admin routes by phone_number_id across organizations", func(t *testing.T) {
		t.Parallel()

		mockServer := newMockWhatsAppServer()
		defer mockServer.close()

		app := newMsgTestApp(t, mockServer)
		homeOrg := testutil.CreateTestOrganization(t, app.DB)
		targetOrg := testutil.CreateTestOrganization(t, app.DB)
		adminRole := testutil.CreateAdminRole(t, app.DB, homeOrg.ID)
		superAdmin := testutil.CreateTestUser(t, app.DB, homeOrg.ID, testutil.WithRoleID(&adminRole.ID), testutil.WithSuperAdmin())

		homeAccount := createTestAccount(t, app, homeOrg.ID)
		homeAccount.PhoneID = "home-phone-id"
		require.NoError(t, app.DB.Model(homeAccount).Update("phone_id", homeAccount.PhoneID).Error)

		targetAccount := &models.WhatsAppAccount{
			BaseModel:          models.BaseModel{ID: uuid.New()},
			OrganizationID:     targetOrg.ID,
			Name:               "target-account",
			PhoneID:            "target-phone-id",
			BusinessID:         "target-business-id",
			AccessToken:        "test-token",
			WebhookVerifyToken: "webhook-token",
			APIVersion:         "v18.0",
			Status:             "active",
		}
		require.NoError(t, app.DB.Create(targetAccount).Error)

		req := testutil.NewJSONRequest(t, map[string]any{
			"phone_number":        "+919876543210",
			"profile_name":        "Cross Org Contact",
			"phone_number_id":     targetAccount.PhoneID,
			"type":                "text",
			"content":             map[string]string{"body": "Route by phone number ID"},
			"whatsapp_message_id": "wamid.cross-org-123",
		})
		testutil.SetFullAuthContext(req, homeOrg.ID, superAdmin.ID, superAdmin.RoleID, true)

		err := app.CreateExternalMessage(req)
		require.NoError(t, err)
		assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

		var contact models.Contact
		require.NoError(t, app.DB.Where("organization_id = ? AND phone_number = ?", targetOrg.ID, "919876543210").First(&contact).Error)
		assert.Equal(t, targetAccount.Name, contact.WhatsAppAccount)

		var msg models.Message
		require.NoError(t, app.DB.Where("contact_id = ?", contact.ID).First(&msg).Error)
		assert.Equal(t, targetOrg.ID, msg.OrganizationID)
		assert.Equal(t, targetAccount.Name, msg.WhatsAppAccount)
		assert.Equal(t, "wamid.cross-org-123", msg.WhatsAppMessageID)

		var homeOrgContacts int64
		require.NoError(t, app.DB.Model(&models.Contact{}).Where("organization_id = ? AND phone_number = ?", homeOrg.ID, "919876543210").Count(&homeOrgContacts).Error)
		assert.Zero(t, homeOrgContacts)
		assert.Len(t, mockServer.sentMessages, 0)
	})

	t.Run("template message renders template body", func(t *testing.T) {
		t.Parallel()

		app := newTestApp(t)
		org := testutil.CreateTestOrganization(t, app.DB)
		adminRole := testutil.CreateAdminRole(t, app.DB, org.ID)
		user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&adminRole.ID))
		account := testutil.CreateTestWhatsAppAccountWith(t, app.DB, org.ID, testutil.WithAccountName("primary"))
		contact := testutil.CreateTestContactWith(t, app.DB, org.ID, testutil.WithContactAccount(account.Name))
		template := testutil.CreateTestTemplate(t, app.DB, org.ID, account.Name)

		template.BodyContent = "Hello {{name}}, your portal is {{portal}}"
		require.NoError(t, app.DB.Model(template).Updates(map[string]any{
			"body_content": template.BodyContent,
		}).Error)

		req := testutil.NewJSONRequest(t, map[string]any{
			"contact_id":          contact.ID.String(),
			"whatsapp_account":    account.Name,
			"type":                "template",
			"template_name":       template.Name,
			"template_params":     map[string]any{"name": "Nitin Jain", "portal": "Income Tax"},
			"content":             map[string]string{"body": "[Template: credentials_v1]"},
			"whatsapp_message_id": "wamid.external-template-123",
		})
		testutil.SetAuthContext(req, org.ID, user.ID)

		err := app.CreateExternalMessage(req)
		require.NoError(t, err)
		assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

		var resp struct {
			Data handlers.MessageResponse `json:"data"`
		}
		require.NoError(t, json.Unmarshal(testutil.GetResponseBody(req), &resp))
		assert.Equal(t, models.MessageTypeTemplate, resp.Data.MessageType)
		assert.Equal(t, map[string]any{"body": "Hello Nitin Jain, your portal is Income Tax"}, resp.Data.Content)

		var msg models.Message
		require.NoError(t, app.DB.First(&msg, resp.Data.ID).Error)
		assert.Equal(t, "Hello Nitin Jain, your portal is Income Tax", msg.Content)

		var updatedContact models.Contact
		require.NoError(t, app.DB.First(&updatedContact, contact.ID).Error)
		assert.Equal(t, "Hello Nitin Jain, your portal is Income Tax", updatedContact.LastMessagePreview)
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
