package handlers

import "testing"

func TestRSVPReminderSkipReason(t *testing.T) {
	cases := []struct {
		name       string
		hasContact bool
		phone      string
		want       string
	}{
		{"sendable", true, "919840445616", ""},
		{"no contact", false, "919840445616", "no contact record"},
		{"no phone", true, "", "no usable phone number"},
		{"unusable phone", true, "abc", "no usable phone number"},
		{"no contact wins over phone", false, "", "no contact record"},
	}
	for _, c := range cases {
		if got := rsvpReminderSkipReason(c.hasContact, c.phone); got != c.want {
			t.Errorf("%s: rsvpReminderSkipReason(%v, %q) = %q, want %q",
				c.name, c.hasContact, c.phone, got, c.want)
		}
	}
}
