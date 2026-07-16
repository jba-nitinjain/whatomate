package handlers

import (
	"testing"
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
