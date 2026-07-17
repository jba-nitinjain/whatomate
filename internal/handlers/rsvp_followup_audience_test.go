package handlers

import (
	"strings"
	"testing"
)

func TestRSVPFollowUpAudienceMissingAnswer(t *testing.T) {
	sql, args, err := rsvpFollowUpAudienceClause(RSVPFollowUpAudienceMissingAnswer, "children_count")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The key comes from user configuration and must never be interpolated.
	if strings.Contains(sql, "children_count") {
		t.Fatalf("answer key must be bound as a parameter, not interpolated into SQL: %s", sql)
	}
	if len(args) != 1 || args[0] != "children_count" {
		t.Fatalf("expected the key bound as the only arg, got %+v", args)
	}
	// Must only chase people who actually replied - chasing non-responders for a
	// follow-up answer is what Reminders is for.
	if !strings.Contains(sql, "responded_at IS NOT NULL") {
		t.Fatalf("missing_answer must be scoped to responders: %s", sql)
	}
}

func TestRSVPFollowUpAudienceMissingAnswerTreatsEmptyAsMissing(t *testing.T) {
	sql, _, err := rsvpFollowUpAudienceClause(RSVPFollowUpAudienceMissingAnswer, "children_count")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// A key present but empty ("") is missing, not answered.
	if !strings.Contains(sql, "IS NULL") || !strings.Contains(sql, "''") {
		t.Fatalf("empty string must count as missing: %s", sql)
	}
}

func TestRSVPFollowUpAudienceMissingAnswerRequiresKey(t *testing.T) {
	if _, _, err := rsvpFollowUpAudienceClause(RSVPFollowUpAudienceMissingAnswer, ""); err == nil {
		t.Fatal("missing_answer without a key must be rejected")
	}
	if _, _, err := rsvpFollowUpAudienceClause(RSVPFollowUpAudienceMissingAnswer, "   "); err == nil {
		t.Fatal("whitespace-only key must be rejected")
	}
}

func TestRSVPFollowUpAudienceResponded(t *testing.T) {
	yes, args, err := rsvpFollowUpAudienceClause(RSVPFollowUpAudienceRespondedYes, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(yes, "attendance") || len(args) != 1 || args[0] != "yes" {
		t.Fatalf("responded_yes must filter attendance = yes, got %s / %+v", yes, args)
	}

	no, args, err := rsvpFollowUpAudienceClause(RSVPFollowUpAudienceRespondedNo, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 1 || args[0] != "no" {
		t.Fatalf("responded_no must filter attendance = no, got %s / %+v", no, args)
	}
}

func TestRSVPFollowUpAudienceNotStarted(t *testing.T) {
	sql, _, err := rsvpFollowUpAudienceClause(RSVPFollowUpAudienceNotStarted, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(sql, "rsvp_started_at IS NULL") {
		t.Fatalf("not_started must match the existing journey definition (rsvp_guests.go:150): %s", sql)
	}
}

func TestRSVPFollowUpAudienceRejectsUnknown(t *testing.T) {
	if _, _, err := rsvpFollowUpAudienceClause(RSVPFollowUpAudience("everyone"), ""); err == nil {
		t.Fatal("unknown audience must be rejected, not silently matched")
	}
}
