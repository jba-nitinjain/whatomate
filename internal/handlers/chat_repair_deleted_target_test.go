package handlers_test

import (
	"encoding/json"
	"testing"

	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func TestApp_ApplyChatRepairCandidates_RestoresDeletedMergeTarget(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	homeOrg := testutil.CreateTestOrganization(t, app.DB)
	targetOrg := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, homeOrg.ID)
	superAdmin := testutil.CreateTestUser(t, app.DB, homeOrg.ID, testutil.WithRoleID(&adminRole.ID), testutil.WithSuperAdmin())

	wrongContact, deletedTargetContact, targetAccount := seedChatRepairDeletedTargetCandidate(t, app, homeOrg.ID, targetOrg.ID)

	previewReq := testutil.NewGETRequest(t)
	testutil.SetQueryParam(previewReq, "limit", 10)
	testutil.SetFullAuthContext(previewReq, homeOrg.ID, superAdmin.ID, superAdmin.RoleID, true)

	err := app.PreviewChatRepairCandidates(previewReq)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(previewReq))

	var previewResp struct {
		Data struct {
			Candidates []handlers.ChatRepairCandidate `json:"candidates"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(testutil.GetResponseBody(previewReq), &previewResp))
	require.Len(t, previewResp.Data.Candidates, 1)
	assert.Equal(t, "merge_required", previewResp.Data.Candidates[0].Action)
	assert.Equal(t, deletedTargetContact.ID.String(), previewResp.Data.Candidates[0].TargetContactID)

	applyReq := testutil.NewJSONRequest(t, handlers.ChatRepairApplyRequest{
		ManualMergeContactIDs: []string{wrongContact.ID.String()},
	})
	testutil.SetFullAuthContext(applyReq, homeOrg.ID, superAdmin.ID, superAdmin.RoleID, true)

	err = app.ApplyChatRepairCandidates(applyReq)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(applyReq))

	var restoredTarget models.Contact
	require.NoError(t, app.DB.First(&restoredTarget, deletedTargetContact.ID).Error)
	assert.Equal(t, targetOrg.ID, restoredTarget.OrganizationID)
	assert.Equal(t, "Legacy sync body", restoredTarget.LastMessagePreview)

	var mergedMessage models.Message
	require.NoError(t, app.DB.Where("whats_app_message_id = ?", "wamid.legacy-chat-repair-deleted-target").First(&mergedMessage).Error)
	assert.Equal(t, deletedTargetContact.ID, mergedMessage.ContactID)
	assert.Equal(t, targetAccount.Name, mergedMessage.WhatsAppAccount)
}
