package handlers_test

import (
	"encoding/json"
	"testing"

	"github.com/nikyjain/whatomate/internal/handlers"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func TestApp_PreviewChatRepairCandidates_DuplicateMoveTargetsBecomeConflicts(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	homeOrg := testutil.CreateTestOrganization(t, app.DB)
	otherSourceOrg := testutil.CreateTestOrganization(t, app.DB)
	targetOrg := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, homeOrg.ID)
	superAdmin := testutil.CreateTestUser(t, app.DB, homeOrg.ID, testutil.WithRoleID(&adminRole.ID), testutil.WithSuperAdmin())

	firstContact, secondContact, targetAccount := seedChatRepairDuplicateMoveCandidates(t, app, homeOrg.ID, otherSourceOrg.ID, targetOrg.ID)

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
	require.Len(t, resp.Data.Candidates, 2)
	assert.EqualValues(t, 0, resp.Data.Summary.MoveCandidates)
	assert.EqualValues(t, 0, resp.Data.Summary.AutoFixableCandidates)
	assert.EqualValues(t, 2, resp.Data.Summary.ConflictCandidates)

	candidatesByID := map[string]handlers.ChatRepairCandidate{}
	for _, candidate := range resp.Data.Candidates {
		candidatesByID[candidate.ContactID] = candidate
	}
	for _, contactID := range []string{firstContact.ID.String(), secondContact.ID.String()} {
		candidate, ok := candidatesByID[contactID]
		require.True(t, ok)
		assert.Equal(t, "conflict", candidate.Action)
		assert.Equal(t, targetOrg.ID.String(), candidate.TargetOrgID)
		assert.Equal(t, targetAccount.Name, candidate.TargetAccount)
		assert.Contains(t, candidate.Reason, "same target phone number")
	}
}

func TestApp_ApplyChatRepairCandidates_RejectsConflictingMoves(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	homeOrg := testutil.CreateTestOrganization(t, app.DB)
	otherSourceOrg := testutil.CreateTestOrganization(t, app.DB)
	targetOrg := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, homeOrg.ID)
	superAdmin := testutil.CreateTestUser(t, app.DB, homeOrg.ID, testutil.WithRoleID(&adminRole.ID), testutil.WithSuperAdmin())

	firstContact, _, _ := seedChatRepairDuplicateMoveCandidates(t, app, homeOrg.ID, otherSourceOrg.ID, targetOrg.ID)

	req := testutil.NewJSONRequest(t, handlers.ChatRepairApplyRequest{
		ContactIDs: []string{firstContact.ID.String()},
	})
	testutil.SetFullAuthContext(req, homeOrg.ID, superAdmin.ID, superAdmin.RoleID, true)

	err := app.ApplyChatRepairCandidates(req)
	require.NoError(t, err)
	testutil.AssertErrorResponse(t, req, fasthttp.StatusConflict, "same target phone number")
}
