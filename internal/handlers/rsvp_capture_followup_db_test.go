package handlers_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/handlers"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/require"
)

// TestFinalizeFollowUp_MergesWithoutClobbering is the regression this branch
// exists for. A live event has 271 members and 207 spouses who already
// responded with attendance + spouse answers. A follow-up flow that only asks
// about children must top up their existing response, not replace it -
// finalizeRSVPFromSession used to overwrite the whole answers map and
// recompute attendance on every completed session, which would have wiped
// every one of those 271 people's attendance and spouse answers the moment a
// follow-up campaign ran.
func TestFinalizeFollowUp_MergesWithoutClobbering(t *testing.T) {
	db := testutil.SetupTestDB(t)
	app := &handlers.App{DB: db, Log: testutil.NopLogger()}
	org := testutil.CreateTestOrganization(t, db)
	contact := testutil.CreateTestContact(t, db, org.ID)

	event := models.RSVPEvent{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		Name:            "Live Event 19/07/2026",
		Status:          models.RSVPEventStatusActive,
		Keyword:         "LIVE19JUL",
		AttendanceField: "attendance",
		CreatedBy:       uuid.New(),
	}
	require.NoError(t, db.Create(&event).Error)

	// Arrange a guest who already responded, days ago.
	respondedAt := time.Now().UTC().AddDate(0, 0, -3)
	resp := models.RSVPResponse{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		RSVPEventID:    event.ID,
		OrganizationID: org.ID,
		ContactID:      contact.ID,
		PhoneNumber:    "919840445616",
		Attendance:     models.RSVPAttendanceYes,
		Answers: models.JSONB{
			"attendance":              "yes",
			"attendance_title":        "Attending",
			"spouse_attendance":       "yes",
			"spouse_attendance_title": "Attending",
			"spouse_mobile":           "919840026019",
		},
		RespondedAt: &respondedAt,
		Source:      models.RSVPGuestSourceContactSelection,
	}
	require.NoError(t, db.Create(&resp).Error)

	// Act: run a follow-up session carrying ONLY children_count.
	session := &models.ChatbotSession{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		ContactID:      contact.ID,
		PhoneNumber:    "919840445616",
		SessionData: models.JSONB{
			"_rsvp_event_id": event.ID.String(),
			"_rsvp_followup": true,
			"children_count": "2",
		},
	}
	app.FinalizeRSVPFromSessionForTest(session)

	var got models.RSVPResponse
	require.NoError(t, db.First(&got, "rsvp_event_id = ? AND contact_id = ?", event.ID, contact.ID).Error)

	require.Equal(t, models.RSVPAttendanceYes, got.Attendance,
		"follow-up must not recompute attendance - a live record was corrupted")
	require.Equal(t, "yes", got.Answers["spouse_attendance"],
		"follow-up must not erase spouse_attendance - a live record was corrupted")
	require.Equal(t, "Attending", got.Answers["spouse_attendance_title"],
		"follow-up must not erase spouse_attendance_title - a live record was corrupted")
	require.Equal(t, "919840026019", got.Answers["spouse_mobile"],
		"follow-up must not erase spouse_mobile - a live record was corrupted")
	require.Equal(t, "Attending", got.Answers["attendance_title"],
		"follow-up must not erase attendance_title - a live record was corrupted")
	require.Equal(t, "2", got.Answers["children_count"],
		"follow-up must add children_count")
	require.NotNil(t, got.RespondedAt)
	require.WithinDuration(t, respondedAt, *got.RespondedAt, time.Second,
		"follow-up must not move responded_at - the guest answered days ago, this is not a new response")
}

// TestFinalizeFollowUp_NoExistingResponse_DoesNotCreate covers the negative
// case: a follow-up for a contact with no existing response must not create
// one. A follow-up with no existing row is a bug (e.g. a stale/incorrect
// audience list), not a new guest, and must not be silently treated as one.
func TestFinalizeFollowUp_NoExistingResponse_DoesNotCreate(t *testing.T) {
	db := testutil.SetupTestDB(t)
	app := &handlers.App{DB: db, Log: testutil.NopLogger()}
	org := testutil.CreateTestOrganization(t, db)
	contact := testutil.CreateTestContact(t, db, org.ID)

	event := models.RSVPEvent{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		Name:            "Live Event 19/07/2026",
		Status:          models.RSVPEventStatusActive,
		Keyword:         "LIVE19JUL2",
		AttendanceField: "attendance",
		CreatedBy:       uuid.New(),
	}
	require.NoError(t, db.Create(&event).Error)

	// No RSVPResponse row exists for this contact/event pair.

	session := &models.ChatbotSession{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		ContactID:      contact.ID,
		PhoneNumber:    "919840445617",
		SessionData: models.JSONB{
			"_rsvp_event_id": event.ID.String(),
			"_rsvp_followup": true,
			"children_count": "1",
		},
	}
	app.FinalizeRSVPFromSessionForTest(session)

	var count int64
	require.NoError(t, db.Model(&models.RSVPResponse{}).
		Where("rsvp_event_id = ? AND contact_id = ?", event.ID, contact.ID).
		Count(&count).Error)
	require.Equal(t, int64(0), count,
		"a follow-up with no existing response must not create one")
}
