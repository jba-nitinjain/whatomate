package handlers_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

// TestListRSVPGuests_SpouseStatusFilterUsesConfiguredKey guards the SQL-side half
// of the spouse-key trap this branch removes: rsvp_guests.go used to filter on
// the literal 'spouse_attendance' key regardless of configuration, so renaming
// the spouse question in the flow builder silently returned zero guests for
// spouse_status filters. It also exercises the parameter binding end-to-end
// against real Postgres - a param-count mismatch in the raw SQL would surface
// here as a query error, not just a silently wrong count.
func TestListRSVPGuests_SpouseStatusFilterUsesConfiguredKey(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&adminRole.ID))

	event := models.RSVPEvent{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		Name:            "Renamed Spouse Question",
		Status:          models.RSVPEventStatusActive,
		Keyword:         "RENAMEDSPOUSE",
		AttendanceField: "attendance",
		CreatedBy:       user.ID,
		HeadcountContributors: models.RSVPHeadcountContributors{
			{Label: "Member attendance", Mode: models.RSVPHeadcountModeAttendance, MatchValues: []string{"yes"}},
			{Label: "Partner attending", AnswerKey: "partner_coming", Mode: models.RSVPHeadcountModeBoolean, MatchValues: []string{"yes", "attending"}},
		},
	}
	require.NoError(t, app.DB.Create(&event).Error)

	newResponse := func(answers models.JSONB) {
		contact := testutil.CreateTestContact(t, app.DB, org.ID)
		resp := models.RSVPResponse{
			BaseModel:      models.BaseModel{ID: uuid.New()},
			RSVPEventID:    event.ID,
			OrganizationID: org.ID,
			ContactID:      contact.ID,
			PhoneNumber:    contact.PhoneNumber,
			Attendance:     models.RSVPAttendanceYes,
			Answers:        answers,
		}
		require.NoError(t, app.DB.Create(&resp).Error)
	}

	// Attending under the CONFIGURED key - must be found.
	newResponse(models.JSONB{"partner_coming": "yes", "spouse_mobile": "919840445616"})
	// Not attending under the configured key - must be excluded.
	newResponse(models.JSONB{"partner_coming": "no"})
	// Attending under the OLD hardcoded key only - must NOT be found. This is
	// the regression check: it proves the filter reads configuration rather
	// than the literal "spouse_attendance" that used to be baked into the SQL.
	newResponse(models.JSONB{"spouse_attendance": "yes", "spouse_mobile": "919840445616"})

	req := testutil.NewGETRequest(t)
	testutil.SetAuthContext(req, org.ID, user.ID)
	testutil.SetPathParam(req, "id", event.ID.String())
	testutil.SetQueryParam(req, "spouse_status", "attending")

	err := app.ListRSVPGuests(req)
	require.NoError(t, err)
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp struct {
		Data struct {
			Total int64 `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(testutil.GetResponseBody(req), &resp))
	require.Equal(t, int64(1), resp.Data.Total, "only the response answering the configured spouse key must match spouse_status=attending")
}

// TestListRSVPGuests_SpouseStatusFilterPendingBranchStaysConsistent guards the
// note at the top of this task's plan: the raw SQL "pending" branch hand-mirrors
// the intentional attending/incomplete-mobile double-count in
// buildRSVPAttendanceBreakdownWithKey (rsvp_tally.go) and must keep doing so
// under the configured key, not just the legacy default.
func TestListRSVPGuests_SpouseStatusFilterPendingBranchStaysConsistent(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	adminRole := testutil.CreateAdminRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&adminRole.ID))

	event := models.RSVPEvent{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		Name:            "Renamed Spouse Pending",
		Status:          models.RSVPEventStatusActive,
		Keyword:         "RENAMEDPENDING",
		AttendanceField: "attendance",
		CreatedBy:       user.ID,
		HeadcountContributors: models.RSVPHeadcountContributors{
			{Label: "Member attendance", Mode: models.RSVPHeadcountModeAttendance, MatchValues: []string{"yes"}},
			{Label: "Partner attending", AnswerKey: "partner_coming", Mode: models.RSVPHeadcountModeBoolean, MatchValues: []string{"yes", "attending"}},
		},
	}
	require.NoError(t, app.DB.Create(&event).Error)

	newResponse := func(answers models.JSONB) {
		contact := testutil.CreateTestContact(t, app.DB, org.ID)
		resp := models.RSVPResponse{
			BaseModel:      models.BaseModel{ID: uuid.New()},
			RSVPEventID:    event.ID,
			OrganizationID: org.ID,
			ContactID:      contact.ID,
			PhoneNumber:    contact.PhoneNumber,
			Attendance:     models.RSVPAttendanceYes,
			Answers:        answers,
		}
		require.NoError(t, app.DB.Create(&resp).Error)
	}

	// Attending under the configured key but with a short (incomplete) mobile
	// number: this must count as pending (double-counted alongside attending,
	// same as the Go breakdown), per the intentional behaviour at
	// rsvp_tally.go:53-61.
	newResponse(models.JSONB{"partner_coming": "yes", "spouse_mobile": "12345"})
	// Never answered the spouse question at all: also pending, via the NOT(...)
	// branch.
	newResponse(models.JSONB{})
	// Attending with a complete mobile number: NOT pending.
	newResponse(models.JSONB{"partner_coming": "yes", "spouse_mobile": "919840445616"})

	req := testutil.NewGETRequest(t)
	testutil.SetAuthContext(req, org.ID, user.ID)
	testutil.SetPathParam(req, "id", event.ID.String())
	testutil.SetQueryParam(req, "spouse_status", "pending")

	err := app.ListRSVPGuests(req)
	require.NoError(t, err)
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp struct {
		Data struct {
			Total int64 `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(testutil.GetResponseBody(req), &resp))
	require.Equal(t, int64(2), resp.Data.Total, "incomplete-mobile attending and never-answered rows must both count as pending")
}
