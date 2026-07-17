package handlers

import "testing"

func TestRSVPShouldBlockDuplicate(t *testing.T) {
	cases := []struct {
		name                       string
		isFollowUp, alreadyReplied bool
		want                       bool
	}{
		{"main rsvp, first time", false, false, false},
		{"main rsvp, already replied - still blocked", false, true, true},
		{"follow-up, already replied - allowed through", true, true, false},
		{"follow-up, never replied", true, false, false},
	}
	for _, c := range cases {
		if got := rsvpShouldBlockDuplicate(c.isFollowUp, c.alreadyReplied); got != c.want {
			t.Errorf("%s: rsvpShouldBlockDuplicate(%v, %v) = %v, want %v",
				c.name, c.isFollowUp, c.alreadyReplied, got, c.want)
		}
	}
}
