package handlers

import (
	"testing"

	"github.com/nikyjain/whatomate/internal/models"
)

func TestBuildRSVPAttendanceBreakdown(t *testing.T) {
	responses := []models.RSVPResponse{
		{Attendance: models.RSVPAttendanceYes, Answers: models.JSONB{"spouse_attendance": "yes", "spouse_mobile": "+91 98765 43210"}},
		{Attendance: models.RSVPAttendanceYes, Answers: models.JSONB{"spouse_attendance": "yes"}},
		{Attendance: models.RSVPAttendanceYes, Answers: models.JSONB{"spouse_attendance": "no"}},
		{Attendance: models.RSVPAttendanceNo, Answers: models.JSONB{}},
		{Attendance: models.RSVPAttendancePending, Answers: models.JSONB{}},
	}

	got := buildRSVPAttendanceBreakdown(responses, "spouse_mobile")
	if got.Member.Attending != 3 || got.Member.NotAttending != 1 || got.Member.Pending != 1 {
		t.Fatalf("unexpected member counts: %+v", got.Member)
	}
	if got.Spouse.Attending != 2 || got.Spouse.NotAttending != 1 || got.Spouse.Pending != 3 {
		t.Fatalf("unexpected spouse counts: %+v", got.Spouse)
	}
}

func TestBuildRSVPAttendanceBreakdownUsesConfiguredSpouseMobileField(t *testing.T) {
	responses := []models.RSVPResponse{{
		Attendance: models.RSVPAttendanceYes,
		Answers: models.JSONB{
			"spouse_attendance_title": "Attending",
			"partner_phone":           "9876543210",
		},
	}}

	got := buildRSVPAttendanceBreakdown(responses, "partner_phone")
	if got.Spouse.Attending != 1 || got.Spouse.Pending != 0 {
		t.Fatalf("unexpected spouse counts: %+v", got.Spouse)
	}
}
