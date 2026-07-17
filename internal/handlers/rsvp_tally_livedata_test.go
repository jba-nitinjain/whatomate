package handlers

import (
	"testing"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/require"
)

// TestRSVPTallyLiveShapedDataMatchesLiveEvent is the live-data regression this
// branch exists for. A real event with 1,276 guests is mid-flight (19/07/2026),
// reading member_attendance: 271 yes and spouse_attendance: 207 yes, and it has
// no HeadcountContributors configured - it was created before that column
// existed. This test seeds a live-shaped RSVPEvent the same way (AttendanceField:
// "attendance", no contributors) plus responses in the exact 271/207/28 shape,
// against a real Postgres database, and calls the same unexported functions
// GetRSVPTally uses (buildRSVPAttendanceBreakdown, legacyHeadcountContributors,
// buildRSVPHeadcount) to prove those existing numbers do not move and that the
// new total_attending comes out to 478 (271 members + 207 spouses).
//
// This lives in package handlers, not handlers_test, because
// buildRSVPAttendanceBreakdown, buildRSVPHeadcount and legacyHeadcountContributors
// are unexported and there is no test-only export for them (unlike
// FinalizeRSVPFromSessionForTest for the capture flow) - exporting them purely
// for this test was rejected in favor of this file living alongside the
// production code it exercises.
func TestRSVPTallyLiveShapedDataMatchesLiveEvent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	org := testutil.CreateTestOrganization(t, db)

	event := models.RSVPEvent{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		Name:            "Live Event 19/07/2026",
		Status:          models.RSVPEventStatusActive,
		Keyword:         "LIVE19JUL",
		AttendanceField: "attendance",
		CreatedBy:       uuid.New(),
		// HeadcountContributors intentionally left unset - mirrors the live
		// event, which was created before this column existed.
	}
	require.NoError(t, db.Create(&event).Error)

	const (
		memberYesWithSpouse    = 207
		memberYesTotal         = 271
		memberYesWithoutSpouse = memberYesTotal - memberYesWithSpouse
		memberNo               = 28
		memberPending          = 5
	)

	responses := make([]models.RSVPResponse, 0, memberYesTotal+memberNo+memberPending)
	addResponse := func(attendance models.RSVPAttendance, answers models.JSONB) {
		// rsvp_responses.contact_id carries a real foreign key (via
		// gorm:"foreignKey:ContactID" on RSVPResponse.Contact, enforced by
		// AutoMigrate), so every response needs a real Contact row - a random
		// ContactID trips the constraint, as TestRSVPModels_Migrate_And_CRUD
		// demonstrates (pre-existing failure on main, unrelated to this branch).
		contact := testutil.CreateTestContact(t, db, org.ID)
		responses = append(responses, models.RSVPResponse{
			BaseModel:      models.BaseModel{ID: uuid.New()},
			RSVPEventID:    event.ID,
			OrganizationID: org.ID,
			ContactID:      contact.ID,
			PhoneNumber:    contact.PhoneNumber,
			Attendance:     attendance,
			Answers:        answers,
		})
	}

	for i := 0; i < memberYesWithSpouse; i++ {
		addResponse(models.RSVPAttendanceYes, models.JSONB{"spouse_attendance": "yes"})
	}
	for i := 0; i < memberYesWithoutSpouse; i++ {
		addResponse(models.RSVPAttendanceYes, models.JSONB{})
	}
	for i := 0; i < memberNo; i++ {
		addResponse(models.RSVPAttendanceNo, models.JSONB{})
	}
	for i := 0; i < memberPending; i++ {
		addResponse(models.RSVPAttendancePending, models.JSONB{})
	}

	require.NoError(t, db.CreateInBatches(responses, 100).Error)

	// Load exactly as GetRSVPTally does (rsvp.go: a.DB.Select("attendance",
	// "answers")...Find(&responses)) rather than reusing the in-memory slice, so
	// this test also exercises the real read path, not just the pure functions.
	var loaded []models.RSVPResponse
	require.NoError(t, db.Select("attendance", "answers").
		Where("organization_id = ? AND rsvp_event_id = ?", org.ID, event.ID).
		Find(&loaded).Error)
	require.Len(t, loaded, len(responses))

	breakdown := buildRSVPAttendanceBreakdown(loaded, event.SpouseMobileField)
	if breakdown.Member.Attending != 271 {
		t.Fatalf("member_attendance moved: got %d yes, want 271 - the live dashboard would shift under the user two days before their event", breakdown.Member.Attending)
	}
	require.Equal(t, 28, breakdown.Member.NotAttending, "existing member_attendance not-attending count must not move")
	require.Equal(t, 207, breakdown.Spouse.Attending, "existing spouse_attendance attending count must not move")

	require.Empty(t, event.HeadcountContributors, "this event must mirror the live event: no configured contributors")
	contributors := legacyHeadcountContributors(event.AttendanceField)

	tallies, total := buildRSVPHeadcount(loaded, contributors)
	require.Equal(t, 478, total, "total_attending must be 271 members + 207 spouses = 478")

	require.Len(t, tallies, 2)
	require.Equal(t, 271, tallies[0].People, "member contributor tally must agree with the member attendance breakdown")
	require.Equal(t, 207, tallies[1].People, "spouse contributor tally must agree with the spouse attendance breakdown")
}
