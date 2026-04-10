package handlers_test

import (
	"testing"

	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func TestApp_ScanChatRepairCandidates_AutoAppliesMove(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	homeOrg := testutil.CreateTestOrganization(t, app.DB)
	targetOrg := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, homeOrg.ID)
	superAdmin := testutil.CreateTestUser(t, app.DB, homeOrg.ID, testutil.WithRoleID(&adminRole.ID), testutil.WithSuperAdmin())

	wrongContact, targetAccount := seedChatRepairCandidate(t, app, homeOrg.ID, targetOrg.ID)

	req := testutil.NewJSONRequest(t, nil)
	testutil.SetFullAuthContext(req, homeOrg.ID, superAdmin.ID, superAdmin.RoleID, true)

	err := app.ScanChatRepairCandidates(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp handlers.ChatRepairScanResult
	testutil.ParseEnvelopeResponse(t, req, &resp)
	assert.EqualValues(t, 1, resp.AutoApplied.ProcessedCandidates)
	assert.EqualValues(t, 1, resp.AutoApplied.UpdatedContacts)
	assert.EqualValues(t, 1, resp.AutoApplied.UpdatedMessages)
	assert.EqualValues(t, 0, resp.AutoApplied.SkippedCandidates)
	assert.Empty(t, resp.Candidates)
	assert.EqualValues(t, 0, resp.Summary.MoveCandidates)

	var updatedContact models.Contact
	require.NoError(t, app.DB.First(&updatedContact, wrongContact.ID).Error)
	assert.Equal(t, targetOrg.ID, updatedContact.OrganizationID)
	assert.Equal(t, targetAccount.Name, updatedContact.WhatsAppAccount)

	var updatedMessage models.Message
	require.NoError(t, app.DB.Where("contact_id = ?", wrongContact.ID).First(&updatedMessage).Error)
	assert.Equal(t, targetOrg.ID, updatedMessage.OrganizationID)
	assert.Equal(t, targetAccount.Name, updatedMessage.WhatsAppAccount)
}

func TestApp_ScanChatRepairCandidates_AutoAppliesSameLocationMessageDrift(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	targetOrg := testutil.CreateTestOrganization(t, app.DB)
	driftOrg := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, targetOrg.ID)
	superAdmin := testutil.CreateTestUser(t, app.DB, targetOrg.ID, testutil.WithRoleID(&adminRole.ID), testutil.WithSuperAdmin())

	contact, targetAccount := seedChatRepairSameLocationMessageDriftCandidate(t, app, targetOrg.ID, driftOrg.ID)

	previewReq := testutil.NewGETRequest(t)
	testutil.SetQueryParam(previewReq, "limit", 10)
	testutil.SetFullAuthContext(previewReq, targetOrg.ID, superAdmin.ID, superAdmin.RoleID, true)

	err := app.PreviewChatRepairCandidates(previewReq)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(previewReq))

	var previewResp struct {
		Summary    handlers.ChatRepairSummary     `json:"summary"`
		Candidates []handlers.ChatRepairCandidate `json:"candidates"`
	}
	testutil.ParseEnvelopeResponse(t, previewReq, &previewResp)
	require.Len(t, previewResp.Candidates, 1)
	assert.Equal(t, contact.ID.String(), previewResp.Candidates[0].ContactID)
	assert.Equal(t, "move", previewResp.Candidates[0].Action)
	assert.Equal(t, "Safe to normalize misrouted messages under this chat to the resolved organization/account", previewResp.Candidates[0].Reason)

	scanReq := testutil.NewJSONRequest(t, nil)
	testutil.SetFullAuthContext(scanReq, targetOrg.ID, superAdmin.ID, superAdmin.RoleID, true)

	err = app.ScanChatRepairCandidates(scanReq)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(scanReq))

	var scanResp handlers.ChatRepairScanResult
	testutil.ParseEnvelopeResponse(t, scanReq, &scanResp)
	assert.EqualValues(t, 1, scanResp.AutoApplied.UpdatedContacts)
	assert.EqualValues(t, 1, scanResp.AutoApplied.UpdatedMessages)
	assert.Empty(t, scanResp.Candidates)

	var updatedContact models.Contact
	require.NoError(t, app.DB.First(&updatedContact, contact.ID).Error)
	assert.Equal(t, targetOrg.ID, updatedContact.OrganizationID)
	assert.Equal(t, targetAccount.Name, updatedContact.WhatsAppAccount)

	var updatedMessage models.Message
	require.NoError(t, app.DB.Where("whats_app_message_id = ?", "wamid.same-location-message-drift").First(&updatedMessage).Error)
	assert.Equal(t, targetOrg.ID, updatedMessage.OrganizationID)
	assert.Equal(t, targetAccount.Name, updatedMessage.WhatsAppAccount)
}

func TestApp_PreviewChatRepairCandidates_SameLocationAlignedChatIsSkipped(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	targetOrg := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, targetOrg.ID)
	superAdmin := testutil.CreateTestUser(t, app.DB, targetOrg.ID, testutil.WithRoleID(&adminRole.ID), testutil.WithSuperAdmin())

	_, _ = seedChatRepairSameLocationAlignedCandidate(t, app, targetOrg.ID)

	req := testutil.NewGETRequest(t)
	testutil.SetQueryParam(req, "limit", 10)
	testutil.SetFullAuthContext(req, targetOrg.ID, superAdmin.ID, superAdmin.RoleID, true)

	err := app.PreviewChatRepairCandidates(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp struct {
		Summary    handlers.ChatRepairSummary     `json:"summary"`
		Candidates []handlers.ChatRepairCandidate `json:"candidates"`
	}
	testutil.ParseEnvelopeResponse(t, req, &resp)
	assert.Empty(t, resp.Candidates)
	assert.EqualValues(t, 0, resp.Summary.MoveCandidates)
}

func TestApp_ScanChatRepairCandidates_LeavesManualMergesUnchanged(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	homeOrg := testutil.CreateTestOrganization(t, app.DB)
	targetOrg := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, homeOrg.ID)
	superAdmin := testutil.CreateTestUser(t, app.DB, homeOrg.ID, testutil.WithRoleID(&adminRole.ID), testutil.WithSuperAdmin())

	wrongContact, _, _ := seedChatRepairMergeCandidate(t, app, homeOrg.ID, targetOrg.ID)

	req := testutil.NewJSONRequest(t, nil)
	testutil.SetFullAuthContext(req, homeOrg.ID, superAdmin.ID, superAdmin.RoleID, true)

	err := app.ScanChatRepairCandidates(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp handlers.ChatRepairScanResult
	testutil.ParseEnvelopeResponse(t, req, &resp)
	assert.EqualValues(t, 0, resp.AutoApplied.UpdatedContacts)
	require.Len(t, resp.Candidates, 1)
	assert.Equal(t, wrongContact.ID.String(), resp.Candidates[0].ContactID)
	assert.Equal(t, "merge_required", resp.Candidates[0].Action)
	assert.EqualValues(t, 1, resp.Summary.MergeRequiredCandidates)
}

func TestApp_ScanChatRepairCandidates_LeavesConflictsUnchanged(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	homeOrg := testutil.CreateTestOrganization(t, app.DB)
	otherSourceOrg := testutil.CreateTestOrganization(t, app.DB)
	targetOrg := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, homeOrg.ID)
	superAdmin := testutil.CreateTestUser(t, app.DB, homeOrg.ID, testutil.WithRoleID(&adminRole.ID), testutil.WithSuperAdmin())

	firstContact, secondContact, _ := seedChatRepairDuplicateMoveCandidates(t, app, homeOrg.ID, otherSourceOrg.ID, targetOrg.ID)

	req := testutil.NewJSONRequest(t, nil)
	testutil.SetFullAuthContext(req, homeOrg.ID, superAdmin.ID, superAdmin.RoleID, true)

	err := app.ScanChatRepairCandidates(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp handlers.ChatRepairScanResult
	testutil.ParseEnvelopeResponse(t, req, &resp)
	assert.EqualValues(t, 0, resp.AutoApplied.UpdatedContacts)
	assert.Len(t, resp.Candidates, 2)
	assert.EqualValues(t, 2, resp.Summary.ConflictCandidates)

	candidatesByID := map[string]handlers.ChatRepairCandidate{}
	for _, candidate := range resp.Candidates {
		candidatesByID[candidate.ContactID] = candidate
	}
	assert.Equal(t, "conflict", candidatesByID[firstContact.ID.String()].Action)
	assert.Equal(t, "conflict", candidatesByID[secondContact.ID.String()].Action)
}
