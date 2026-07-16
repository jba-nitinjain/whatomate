package handlers

import "testing"

func TestNormalizeRSVPReminderPhone(t *testing.T) {
	cases := []struct{ in, want string }{
		{"919840445616", "919840445616"},
		{"9840445616", "919840445616"},   // bare 10-digit gets 91, matching rsvp_capture.go
		{"+91 98404 45616", "919840445616"},
		{"+91-98404-45616", "919840445616"},
		{" 919840445616 ", "919840445616"},
		{"", ""},
		{"abc", ""},
		{"12345", "12345"},               // too short to be a bare Indian mobile: left alone
	}
	for _, c := range cases {
		if got := normalizeRSVPReminderPhone(c.in); got != c.want {
			t.Errorf("normalizeRSVPReminderPhone(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDedupeRSVPReminderRowsFirstWins(t *testing.T) {
	type row struct{ id, phone string }
	rows := []row{
		{"a", "919840445616"},
		{"b", "9840445616"},      // same person, different format
		{"c", "+91 98404 45616"}, // same person again
		{"d", "919999999999"},
	}

	kept, dropped := dedupeRSVPReminderRows(rows, func(r row) string { return r.phone })

	if len(kept) != 2 {
		t.Fatalf("expected 2 kept, got %d: %+v", len(kept), kept)
	}
	if kept[0].id != "a" || kept[1].id != "d" {
		t.Fatalf("expected first-wins a,d; got %+v", kept)
	}
	if len(dropped) != 2 {
		t.Fatalf("expected 2 dropped, got %d: %+v", len(dropped), dropped)
	}
}

func TestDedupeRSVPReminderRowsKeepsUnnormalizable(t *testing.T) {
	// Rows with no digits must survive dedupe so the caller can reject them
	// with a visible reason instead of them vanishing silently.
	type row struct{ id, phone string }
	rows := []row{{"a", ""}, {"b", "abc"}}

	kept, dropped := dedupeRSVPReminderRows(rows, func(r row) string { return r.phone })

	if len(kept) != 2 {
		t.Fatalf("unnormalizable rows must be kept, got %d: %+v", len(kept), kept)
	}
	if len(dropped) != 0 {
		t.Fatalf("expected nothing dropped, got %+v", dropped)
	}
}
