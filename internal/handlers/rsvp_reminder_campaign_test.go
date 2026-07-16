package handlers

import (
	"testing"

	"github.com/nikyjain/whatomate/internal/models"
)

func TestRSVPReminderCampaignOutcomeAllFailed(t *testing.T) {
	cases := []struct {
		name       string
		sent, fail int
		total      int
		want       string
	}{
		{"all failed", 0, 1008, 1008, "failed"},
		{"partial", 900, 108, 1008, "completed_with_errors"},
		{"clean", 1008, 0, 1008, "completed"},
		{"empty", 0, 0, 0, "completed"},
	}
	for _, c := range cases {
		if got := rsvpReminderCampaignOutcome(c.sent, c.fail, c.total); got != c.want {
			t.Errorf("%s: rsvpReminderCampaignOutcome(%d,%d,%d) = %q, want %q",
				c.name, c.sent, c.fail, c.total, got, c.want)
		}
	}
}

// TestRSVPReminderMediaValidationError_RejectsMissingStagingID pins the fix for
// the 15/07/2026 incident: SendRSVPReminders (rsvp_reminders.go) now calls
// this function before createRSVPReminderCampaign so a VIDEO/IMAGE/DOCUMENT
// header template with no staged file is rejected up front with a clean,
// user-fixable 400 - instead of reaching createRSVPReminderCampaign and
// failing 1008 times against Meta with error 132012, or (post-fix, without
// this pre-check) leaking createRSVPReminderCampaign's raw infrastructure
// errors to the client. Without this check the handler would fall through to
// createRSVPReminderCampaign's own validateCampaignReadyForStart gate deep
// inside a transaction-adjacent path, which this test would not catch failing.
func TestRSVPReminderMediaValidationError_RejectsMissingStagingID(t *testing.T) {
	cases := []struct {
		name       string
		headerType string
		stagingID  string
		wantErr    string
	}{
		{"video no staging id", "VIDEO", "", "template requires video header media. Configure campaign media before starting"},
		{"image no staging id", "IMAGE", "", "template requires image header media. Configure campaign media before starting"},
		{"document no staging id", "DOCUMENT", "", "template requires document header media. Configure campaign media before starting"},
		{"video with staging id ok", "VIDEO", "abc123", ""},
		{"whitespace staging id rejected", "VIDEO", "   ", "template requires video header media. Configure campaign media before starting"},
		{"text header needs nothing", "TEXT", "", ""},
		{"empty header type needs nothing", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			template := &models.Template{HeaderType: c.headerType}
			err := rsvpReminderMediaValidationError(template, c.stagingID)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("rsvpReminderMediaValidationError(%q, %q) = %v, want nil", c.headerType, c.stagingID, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("rsvpReminderMediaValidationError(%q, %q) = nil, want error %q", c.headerType, c.stagingID, c.wantErr)
			}
			if err.Error() != c.wantErr {
				t.Fatalf("rsvpReminderMediaValidationError(%q, %q) = %q, want %q", c.headerType, c.stagingID, err.Error(), c.wantErr)
			}
		})
	}
}
