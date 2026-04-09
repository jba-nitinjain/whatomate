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

func TestApp_PreviewChatRepairCandidates(t *testing.T) {
	t.Parallel()

	t.Run("super admin sees move candidate", func(t *testing.T) {
		t.Parallel()

		app := newTestApp(t)
		homeOrg := testutil.CreateTestOrganization(t, app.DB)
		targetOrg := testutil.CreateTestOrganization(t, app.DB)
		adminRole := testutil.CreateAdminRole(t, app.DB, homeOrg.ID)
		superAdmin := testutil.CreateTestUser(t, app.DB, homeOrg.ID, testutil.WithRoleID(&adminRole.ID), testutil.WithSuperAdmin())

		wrongContact, targetAccount := seedChatRepairCandidate(t, app, homeOrg.ID, targetOrg.ID)

		req := testutil.NewGETRequest(t)
		testutil.SetQueryParam(req, "limit", 10)
		testutil.SetFullAuthContext(req, homeOrg.ID, superAdmin.ID, superAdmin.RoleID, true)

		err := app.PreviewChatRepairCandidates(req)
		require.NoError(t, err)
		assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

		var resp struct {
			Data struct {
				Summary    handlers.ChatRepairSummary     `json:"summary"`
				Candidates []handlers.ChatRepairCandidate `json:"candidates"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(testutil.GetResponseBody(req), &resp))
		require.Len(t, resp.Data.Candidates, 1)
		assert.Equal(t, handlers.ChatRepairCandidate{
			ContactID:            wrongContact.ID.String(),
			PhoneNumber:          wrongContact.PhoneNumber,
			ProfileName:          wrongContact.ProfileName,
			CurrentOrgID:         homeOrg.ID.String(),
			CurrentOrgName:       homeOrg.Name,
			CurrentAccount:       "home-account",
			TargetOrgID:          targetOrg.ID.String(),
			TargetOrgName:        targetOrg.Name,
			TargetAccount:        targetAccount.Name,
			Action:               "move",
			Reason:               "Safe to move this chat to the resolved organization/account",
			AffectedMessageCount: 1,
			PhoneNumberID:        targetAccount.PhoneID,
		}, normalizeChatRepairCandidate(resp.Data.Candidates[0]))
		assert.EqualValues(t, 1, resp.Data.Summary.ScannedContacts)
		assert.EqualValues(t, 1, resp.Data.Summary.MoveCandidates)
		assert.EqualValues(t, 1, resp.Data.Summary.AutoFixableCandidates)
	})

	t.Run("non super admin is forbidden", func(t *testing.T) {
		t.Parallel()

		app := newTestApp(t)
		org := testutil.CreateTestOrganization(t, app.DB)
		adminRole := testutil.CreateAdminRole(t, app.DB, org.ID)
		user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&adminRole.ID))

		req := testutil.NewGETRequest(t)
		testutil.SetFullAuthContext(req, org.ID, user.ID, user.RoleID, false)

		err := app.PreviewChatRepairCandidates(req)
		require.NoError(t, err)
		testutil.AssertErrorResponse(t, req, fasthttp.StatusForbidden, "Only super admins can access chat repair")
	})
}

func TestApp_ApplyChatRepairCandidates(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	homeOrg := testutil.CreateTestOrganization(t, app.DB)
	targetOrg := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, homeOrg.ID)
	superAdmin := testutil.CreateTestUser(t, app.DB, homeOrg.ID, testutil.WithRoleID(&adminRole.ID), testutil.WithSuperAdmin())

	wrongContact, targetAccount := seedChatRepairCandidate(t, app, homeOrg.ID, targetOrg.ID)

	req := testutil.NewJSONRequest(t, handlers.ChatRepairApplyRequest{
		ContactIDs: []string{wrongContact.ID.String()},
	})
	testutil.SetFullAuthContext(req, homeOrg.ID, superAdmin.ID, superAdmin.RoleID, true)

	err := app.ApplyChatRepairCandidates(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp struct {
		Data handlers.ChatRepairApplyResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(testutil.GetResponseBody(req), &resp))
	assert.EqualValues(t, 1, resp.Data.ProcessedCandidates)
	assert.EqualValues(t, 1, resp.Data.UpdatedContacts)
	assert.EqualValues(t, 1, resp.Data.UpdatedMessages)
	assert.EqualValues(t, 0, resp.Data.SkippedCandidates)

	var updatedContact models.Contact
	require.NoError(t, app.DB.First(&updatedContact, wrongContact.ID).Error)
	assert.Equal(t, targetOrg.ID, updatedContact.OrganizationID)
	assert.Equal(t, targetAccount.Name, updatedContact.WhatsAppAccount)
	assert.Equal(t, "Legacy sync body", updatedContact.LastMessagePreview)

	var updatedMessage models.Message
	require.NoError(t, app.DB.Where("contact_id = ?", wrongContact.ID).First(&updatedMessage).Error)
	assert.Equal(t, targetOrg.ID, updatedMessage.OrganizationID)
	assert.Equal(t, targetAccount.Name, updatedMessage.WhatsAppAccount)
}

func seedChatRepairCandidate(t *testing.T, app *handlers.App, homeOrgID, targetOrgID uuid.UUID) (*models.Contact, *models.WhatsAppAccount) {
	t.Helper()

	homeAccount := &models.WhatsAppAccount{
		BaseModel:          models.BaseModel{ID: uuid.New()},
		OrganizationID:     homeOrgID,
		Name:               "home-account",
		PhoneID:            "home-phone-id",
		BusinessID:         "home-business-id",
		AccessToken:        "test-token",
		WebhookVerifyToken: "verify-home",
		APIVersion:         "v18.0",
		Status:             "active",
	}
	targetAccount := &models.WhatsAppAccount{
		BaseModel:          models.BaseModel{ID: uuid.New()},
		OrganizationID:     targetOrgID,
		Name:               "target-account",
		PhoneID:            "target-phone-id",
		BusinessID:         "target-business-id",
		AccessToken:        "test-token",
		WebhookVerifyToken: "verify-target",
		APIVersion:         "v18.0",
		Status:             "active",
	}
	require.NoError(t, app.DB.Create(homeAccount).Error)
	require.NoError(t, app.DB.Create(targetAccount).Error)

	wrongContact := testutil.CreateTestContactWith(
		t,
		app.DB,
		homeOrgID,
		testutil.WithContactAccount(homeAccount.Name),
		testutil.WithPhoneNumber("919999000111"),
	)
	require.NoError(t, app.DB.Model(wrongContact).Updates(map[string]any{
		"profile_name":         "Legacy Contact",
		"last_message_preview": "Wrong preview",
		"whats_app_account":    homeAccount.Name,
	}).Error)

	message := &models.Message{
		BaseModel:         models.BaseModel{ID: uuid.New()},
		OrganizationID:    homeOrgID,
		WhatsAppAccount:   homeAccount.Name,
		ContactID:         wrongContact.ID,
		Direction:         models.DirectionOutgoing,
		MessageType:       models.MessageTypeText,
		Content:           "Legacy sync body",
		Status:            models.MessageStatusSent,
		Metadata:          models.JSONB{"source": "external_api", "source_system": "aws_lambda", "phone_number_id": targetAccount.PhoneID},
		WhatsAppMessageID: "wamid.legacy-chat-repair",
	}
	require.NoError(t, app.DB.Create(message).Error)

	return wrongContact, targetAccount
}

func normalizeChatRepairCandidate(candidate handlers.ChatRepairCandidate) handlers.ChatRepairCandidate {
	candidate.LastMessageAt = nil
	return candidate
}
