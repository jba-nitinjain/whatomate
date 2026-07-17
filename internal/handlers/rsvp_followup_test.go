package handlers

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

// TestLoadRSVPFollowUpGuests pins the missing_answer audience against a real
// database: two guests answered yes but never filled in children_count (one
// key absent entirely, one present-but-empty), one guest answered it, and one
// guest never responded at all. Only the first two must come back - answering
// excludes a guest, and never replying is what Reminders (not Follow-up) is
// for, per rsvpFollowUpAudienceClause's RSVPFollowUpAudienceMissingAnswer case.
func TestLoadRSVPFollowUpGuests(t *testing.T) {
	db := testutil.SetupTestDB(t)
	app := &App{DB: db, Log: testutil.NopLogger()}
	org := testutil.CreateTestOrganization(t, db)
	contactAbsent := testutil.CreateTestContact(t, db, org.ID)
	contactAnswered := testutil.CreateTestContact(t, db, org.ID)
	contactEmpty := testutil.CreateTestContact(t, db, org.ID)
	contactNotStarted := testutil.CreateTestContact(t, db, org.ID)
	event := models.RSVPEvent{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		Name:           "Follow-up Preview Test",
		Status:         models.RSVPEventStatusActive,
		AccessMode:     models.RSVPAccessModeGuestList,
		CreatedBy:      uuid.New(),
	}
	require.NoError(t, db.Create(&event).Error)

	now := time.Now().UTC()
	// (a) responded yes, no children_count key at all.
	responseAbsentKey := models.RSVPResponse{
		BaseModel: models.BaseModel{ID: uuid.New()}, RSVPEventID: event.ID, OrganizationID: org.ID,
		ContactID: contactAbsent.ID, PhoneNumber: contactAbsent.PhoneNumber,
		Attendance: models.RSVPAttendanceYes, Answers: models.JSONB{"attendance": "yes"},
		RespondedAt: &now, Source: models.RSVPGuestSourceContactSelection,
	}
	// (b) responded yes, answered children_count.
	responseAnswered := models.RSVPResponse{
		BaseModel: models.BaseModel{ID: uuid.New()}, RSVPEventID: event.ID, OrganizationID: org.ID,
		ContactID: contactAnswered.ID, PhoneNumber: contactAnswered.PhoneNumber,
		Attendance: models.RSVPAttendanceYes, Answers: models.JSONB{"attendance": "yes", "children_count": "2"},
		RespondedAt: &now, Source: models.RSVPGuestSourceContactSelection,
	}
	// (c) responded yes, children_count present but empty.
	responseEmptyValue := models.RSVPResponse{
		BaseModel: models.BaseModel{ID: uuid.New()}, RSVPEventID: event.ID, OrganizationID: org.ID,
		ContactID: contactEmpty.ID, PhoneNumber: contactEmpty.PhoneNumber,
		Attendance: models.RSVPAttendanceYes, Answers: models.JSONB{"attendance": "yes", "children_count": ""},
		RespondedAt: &now, Source: models.RSVPGuestSourceContactSelection,
	}
	// (d) never started - not a follow-up candidate at all, missing_answer is
	// scoped to responders (rsvpFollowUpAudienceClause).
	responseNeverStarted := models.RSVPResponse{
		BaseModel: models.BaseModel{ID: uuid.New()}, RSVPEventID: event.ID, OrganizationID: org.ID,
		ContactID: contactNotStarted.ID, PhoneNumber: contactNotStarted.PhoneNumber,
		Attendance: models.RSVPAttendancePending, Answers: models.JSONB{}, Source: models.RSVPGuestSourceContactSelection,
	}
	require.NoError(t, db.Create(&[]models.RSVPResponse{responseAbsentKey, responseAnswered, responseEmptyValue, responseNeverStarted}).Error)

	rows, err := app.loadRSVPFollowUpGuests(org.ID, event.ID, RSVPFollowUpAudienceMissingAnswer, "children_count")
	require.NoError(t, err)

	gotIDs := make(map[uuid.UUID]bool, len(rows))
	for _, row := range rows {
		gotIDs[row.ID] = true
		assert.NotNil(t, row.Contact, "loader must preload Contact so callers can build skip reasons")
	}

	assert.Len(t, rows, 2, "expected exactly the two guests who responded but left children_count missing")
	assert.True(t, gotIDs[responseAbsentKey.ID], "guest with no children_count key at all must be included")
	assert.True(t, gotIDs[responseEmptyValue.ID], "guest with an empty children_count value must be included")
	assert.False(t, gotIDs[responseAnswered.ID], "guest who answered children_count must be excluded")
	assert.False(t, gotIDs[responseNeverStarted.ID], "guest who never responded must be excluded")
}

// TestRSVPFollowUpEligibilitySkipsMissingContactAndPhone pins that the preview
// reuses rsvpReminderSkipReason - the exact predicate the reminder send path
// uses - rather than a second, possibly-drifting copy. Nil-contact and
// unusable-phone rows must show up as skipped, not silently counted eligible.
func TestRSVPFollowUpEligibilitySkipsMissingContactAndPhone(t *testing.T) {
	contact := &models.Contact{ProfileName: "Asha"}
	rows := []rsvpGuestRosterRow{
		{RSVPResponse: models.RSVPResponse{BaseModel: models.BaseModel{ID: uuid.New()}, PhoneNumber: "919840445610", Contact: contact}},
		{RSVPResponse: models.RSVPResponse{BaseModel: models.BaseModel{ID: uuid.New()}, PhoneNumber: "919840445611", Contact: nil}},
		{RSVPResponse: models.RSVPResponse{BaseModel: models.BaseModel{ID: uuid.New()}, PhoneNumber: "", Contact: contact}},
	}

	recipients, skipped := rsvpFollowUpEligibility(rows)

	require.Len(t, recipients, 1)
	assert.Equal(t, rows[0].ID, recipients[0].ID)
	require.Len(t, skipped, 2)
	reasons := map[uuid.UUID]string{}
	for _, s := range skipped {
		reasons[s.ResponseID] = s.Reason
	}
	assert.Equal(t, "no contact record", reasons[rows[1].ID])
	assert.Equal(t, "no usable phone number", reasons[rows[2].ID])
}

// TestRSVPFollowUpEligibilityDedupesDuplicatePhones mirrors
// TestRSVPReminderEligibilityDedupesDuplicatePhones (rsvp_reminder_skips_test.go):
// two rows sharing a normalized phone must only ever count as one eligible
// recipient, the same way the reminder path already guarantees.
func TestRSVPFollowUpEligibilityDedupesDuplicatePhones(t *testing.T) {
	contact := &models.Contact{ProfileName: "Priya"}
	rows := []rsvpGuestRosterRow{
		{RSVPResponse: models.RSVPResponse{BaseModel: models.BaseModel{ID: uuid.New()}, PhoneNumber: "9840445616", Contact: contact}},
		{RSVPResponse: models.RSVPResponse{BaseModel: models.BaseModel{ID: uuid.New()}, PhoneNumber: "919840445616", Contact: contact}},
	}

	recipients, skipped := rsvpFollowUpEligibility(rows)

	require.Len(t, recipients, 1)
	require.Len(t, skipped, 1)
	assert.Equal(t, "duplicate phone number", skipped[0].Reason)
	assert.Equal(t, rows[1].ID, skipped[0].ResponseID)
}

// TestRSVPFollowUpPreviewErrorEnvelope mirrors
// TestRSVPReminderCampaignErrorEnvelope (rsvp_reminder_campaign_test.go):
// loadRSVPFollowUpGuests conflates two different error classes - an
// audience-clause validation error (bad/missing answer_key, unknown
// audience) that is safe and useful to show the user, and a genuine DB Find
// error (timeout, connection drop) that is not. This asserts
// rsvpFollowUpPreviewErrorEnvelope tells them apart via errors.As - a
// user-facing error surfaces its own message as a 400, everything else stays
// a generic 500 rather than leaking driver-level text - and that the
// distinction survives an extra layer of %w wrapping.
func TestRSVPFollowUpPreviewErrorEnvelope(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "user-facing audience validation error surfaces its own message as a 400",
			err:        rsvpUserFacingError{fmt.Errorf("missing_answer requires an answer key")},
			wantStatus: fasthttp.StatusBadRequest,
			wantMsg:    "missing_answer requires an answer key",
		},
		{
			name:       "user-facing error wrapped further by %w is still detected",
			err:        fmt.Errorf("load follow-up guests: %w", rsvpUserFacingError{fmt.Errorf("unknown follow-up audience: \"bogus\"")}),
			wantStatus: fasthttp.StatusBadRequest,
			wantMsg:    `unknown follow-up audience: "bogus"`,
		},
		{
			name:       "plain infrastructure error stays a generic 500, not leaked",
			err:        fmt.Errorf("pq: connection reset by peer"),
			wantStatus: fasthttp.StatusInternalServerError,
			wantMsg:    "Failed to preview follow-up",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, msg := rsvpFollowUpPreviewErrorEnvelope(c.err)
			if status != c.wantStatus || msg != c.wantMsg {
				t.Fatalf("rsvpFollowUpPreviewErrorEnvelope(%v) = (%d, %q), want (%d, %q)", c.err, status, msg, c.wantStatus, c.wantMsg)
			}
		})
	}
}

// TestLoadRSVPFollowUpGuests_InvalidAudienceIsUserFacing exercises the real
// production call chain (loadRSVPFollowUpGuests ->
// rsvpFollowUpPreviewErrorEnvelope) end to end, not a synthetic error - the
// test the task asked for. An unknown audience never reaches the DB (the
// clause is validated before a.DB is touched), so this needs no database.
// Without the fix, loadRSVPFollowUpGuests returns the validation error
// unwrapped: the errors.As assertion fails, and
// rsvpFollowUpPreviewErrorEnvelope would have nothing to classify - proving
// this test would fail if the wrapping in rsvp_followup.go were reverted.
func TestLoadRSVPFollowUpGuests_InvalidAudienceIsUserFacing(t *testing.T) {
	app := &App{}
	orgID, eventID := uuid.New(), uuid.New()

	_, err := app.loadRSVPFollowUpGuests(orgID, eventID, RSVPFollowUpAudience("bogus"), "")
	require.Error(t, err)

	var userErr rsvpUserFacingError
	if !errors.As(err, &userErr) {
		t.Fatalf("loadRSVPFollowUpGuests(bogus audience) = %v, want a rsvpUserFacingError so PreviewRSVPFollowUp can surface it as a 400 instead of a generic 500", err)
	}

	status, msg := rsvpFollowUpPreviewErrorEnvelope(err)
	assert.Equal(t, fasthttp.StatusBadRequest, status, "an audience validation error must not fall back to the generic 500")
	assert.Equal(t, `unknown follow-up audience: "bogus"`, msg)
}
