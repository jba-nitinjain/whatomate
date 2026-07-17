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

func TestBuildRSVPHeadcount(t *testing.T) {
	contributors := models.RSVPHeadcountContributors{
		{Label: "Member", AnswerKey: "attendance", Mode: models.RSVPHeadcountModeBoolean, MatchValues: []string{"yes"}},
		{Label: "Spouse", AnswerKey: "spouse_attendance", Mode: models.RSVPHeadcountModeBoolean, MatchValues: []string{"yes"}},
		{Label: "Children", AnswerKey: "children_count", Mode: models.RSVPHeadcountModeNumeric},
	}
	responses := []models.RSVPResponse{
		{Attendance: models.RSVPAttendanceYes, Answers: models.JSONB{"attendance": "yes", "spouse_attendance": "yes", "children_count": "2"}},
		{Attendance: models.RSVPAttendanceYes, Answers: models.JSONB{"attendance": "yes", "spouse_attendance": "no", "children_count": "1"}},
		{Attendance: models.RSVPAttendanceNo, Answers: models.JSONB{"attendance": "no"}},
	}

	tallies, total := buildRSVPHeadcount(responses, contributors)

	if len(tallies) != 3 {
		t.Fatalf("expected 3 tallies, got %d", len(tallies))
	}
	if tallies[0].People != 2 {
		t.Errorf("member people = %d, want 2", tallies[0].People)
	}
	if tallies[1].People != 1 {
		t.Errorf("spouse people = %d, want 1", tallies[1].People)
	}
	if tallies[2].People != 3 {
		t.Errorf("children people = %d, want 3", tallies[2].People)
	}
	// 2 members + 1 spouse + 3 children
	if total != 6 {
		t.Errorf("total = %d, want 6", total)
	}
}

func TestBuildRSVPHeadcountFlagsBadNumbers(t *testing.T) {
	contributors := models.RSVPHeadcountContributors{
		{Label: "Children", AnswerKey: "children_count", Mode: models.RSVPHeadcountModeNumeric},
	}
	responses := []models.RSVPResponse{
		{Attendance: models.RSVPAttendanceYes, Answers: models.JSONB{"children_count": "2"}},
		{Attendance: models.RSVPAttendanceYes, Answers: models.JSONB{"children_count": "lots"}},
		{Attendance: models.RSVPAttendanceYes, Answers: models.JSONB{"children_count": "50"}},
	}

	tallies, total := buildRSVPHeadcount(responses, contributors)

	if tallies[0].Unparseable != 1 {
		t.Errorf("unparseable = %d, want 1", tallies[0].Unparseable)
	}
	if tallies[0].NeedsReview != 1 {
		t.Errorf("needs review = %d, want 1", tallies[0].NeedsReview)
	}
	// "lots" contributes 0, and is not silently dropped from the flag counts.
	if total != 52 {
		t.Errorf("total = %d, want 52", total)
	}
}

func TestBuildRSVPHeadcountNoContributorsIsZero(t *testing.T) {
	tallies, total := buildRSVPHeadcount([]models.RSVPResponse{
		{Attendance: models.RSVPAttendanceYes, Answers: models.JSONB{"attendance": "yes"}},
	}, models.RSVPHeadcountContributors{})

	if len(tallies) != 0 || total != 0 {
		t.Fatalf("no contributors must yield no tallies and zero total, got %+v / %d", tallies, total)
	}
}
