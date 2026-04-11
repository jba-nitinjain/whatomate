package handlers_test

import (
	"encoding/json"
	"testing"

	"github.com/nikyjain/whatomate/internal/handlers"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
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
		require.Len(t, resp.Data.Candidates[0].SampleMessages, 1)
		assert.Equal(t, "outgoing", resp.Data.Candidates[0].SampleMessages[0].Direction)
		assert.Equal(t, "text", resp.Data.Candidates[0].SampleMessages[0].MessageType)
		assert.Equal(t, "Legacy sync body", resp.Data.Candidates[0].SampleMessages[0].Preview)
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

	t.Run("account evidence can resolve candidate without phone_number_id metadata", func(t *testing.T) {
		t.Parallel()

		app := newTestApp(t)
		homeOrg := testutil.CreateTestOrganization(t, app.DB)
		targetOrg := testutil.CreateTestOrganization(t, app.DB)
		adminRole := testutil.CreateAdminRole(t, app.DB, homeOrg.ID)
		superAdmin := testutil.CreateTestUser(t, app.DB, homeOrg.ID, testutil.WithRoleID(&adminRole.ID), testutil.WithSuperAdmin())

		wrongContact, targetAccount := seedChatRepairAccountEvidenceCandidate(t, app, homeOrg.ID, targetOrg.ID)

		req := testutil.NewGETRequest(t)
		testutil.SetQueryParam(req, "limit", 10)
		testutil.SetFullAuthContext(req, homeOrg.ID, superAdmin.ID, superAdmin.RoleID, true)

		err := app.PreviewChatRepairCandidates(req)
		require.NoError(t, err)
		assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

		var resp struct {
			Data struct {
				Candidates []handlers.ChatRepairCandidate `json:"candidates"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(testutil.GetResponseBody(req), &resp))
		require.Len(t, resp.Data.Candidates, 1)
		assert.Equal(t, wrongContact.ID.String(), resp.Data.Candidates[0].ContactID)
		assert.Equal(t, targetOrg.ID.String(), resp.Data.Candidates[0].TargetOrgID)
		assert.Equal(t, targetAccount.Name, resp.Data.Candidates[0].TargetAccount)
		assert.Equal(t, "move", resp.Data.Candidates[0].Action)
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

func TestApp_ApplyChatRepairCandidates_ManualMerge(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	homeOrg := testutil.CreateTestOrganization(t, app.DB)
	targetOrg := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, homeOrg.ID)
	superAdmin := testutil.CreateTestUser(t, app.DB, homeOrg.ID, testutil.WithRoleID(&adminRole.ID), testutil.WithSuperAdmin())

	wrongContact, targetContact, targetAccount := seedChatRepairMergeCandidate(t, app, homeOrg.ID, targetOrg.ID)

	req := testutil.NewJSONRequest(t, handlers.ChatRepairApplyRequest{
		ManualMergeContactIDs: []string{wrongContact.ID.String()},
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

	var mergedMessage models.Message
	require.NoError(t, app.DB.Where("whats_app_message_id = ?", "wamid.legacy-chat-repair-merge").First(&mergedMessage).Error)
	assert.Equal(t, targetContact.ID, mergedMessage.ContactID)
	assert.Equal(t, targetOrg.ID, mergedMessage.OrganizationID)
	assert.Equal(t, targetAccount.Name, mergedMessage.WhatsAppAccount)

	var archivedContact models.Contact
	require.NoError(t, app.DB.Unscoped().First(&archivedContact, wrongContact.ID).Error)
	require.NotNil(t, archivedContact.DeletedAt)
	assert.True(t, archivedContact.DeletedAt.Valid)

	var refreshedTarget models.Contact
	require.NoError(t, app.DB.First(&refreshedTarget, targetContact.ID).Error)
	assert.Equal(t, "Legacy sync body", refreshedTarget.LastMessagePreview)
}

func normalizeChatRepairCandidate(candidate handlers.ChatRepairCandidate) handlers.ChatRepairCandidate {
	candidate.LastMessageAt = nil
	candidate.SampleMessages = nil
	return candidate
}
