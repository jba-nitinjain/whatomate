package handlers

import "testing"

func TestParseHeadcountValue(t *testing.T) {
	cases := []struct {
		in    string
		value int
		ok    bool
	}{
		{"3", 3, true},
		{"0", 0, true},
		{"3 kids", 3, true},
		{"we are bringing 2 children", 2, true},
		{"three", 3, true},
		{"Three", 3, true},
		{"TWO", 2, true},
		{"zero", 0, true},
		{"ten", 10, true},
		{"", 0, true},
		{"   ", 0, true},
		{"no", 0, true},
		{"none", 0, true},
		{"nil", 0, true},
		{"-1", 0, false},   // nonsense: flagged for a human, never silently turned into 1
		{"-3 kids", 0, false},
		{"999", 999, true}, // parsed; flagged separately by headcountNeedsReview
		{"abc", 0, false},
		{"many", 0, false},
		{"a few", 0, false},
		// Ambiguous digit answers: more than one distinct number means a human
		// decides, not a silent pick of the first digit run.
		{"1 or 2", 0, false},
		{"2-3", 0, false},
		{"1 2", 0, false},
		// A repeated mention of the same number is not ambiguous.
		{"2 kids, 2 of them", 2, true},
		// Digits take precedence over words when both are present.
		{"2 kids", 2, true},
		// "none at all" must not become 1 via a substring match on "one" inside
		// "none" - word matching must be word-boundary aware.
		{"none at all", 0, false},
		// Cross-ambiguity between a digit reading and a conflicting word
		// reading must not be silently resolved in favor of the digit.
		{"2 or three", 0, false},
		{"one, actually 2", 0, false},
		// A digit and a word that agree on the same value are not ambiguous.
		{"1 or one", 1, true},
	}
	for _, c := range cases {
		value, ok := parseHeadcountValue(c.in)
		if value != c.value || ok != c.ok {
			t.Errorf("parseHeadcountValue(%q) = (%d, %v), want (%d, %v)", c.in, value, ok, c.value, c.ok)
		}
	}
}

// TestParseHeadcountValue_WordAmbiguityIsDeterministic guards against the
// regression this fix addresses: headcountWords used to be a Go map, and Go
// map iteration order is randomized, so an answer containing two number words
// ("one or two") could parse to a different number on different runs. Running
// the assertion many times makes sure a map-order regression can't pass by
// luck.
func TestParseHeadcountValue_WordAmbiguityIsDeterministic(t *testing.T) {
	for i := 0; i < 100; i++ {
		value, ok := parseHeadcountValue("one or two")
		if value != 0 || ok != false {
			t.Fatalf("iteration %d: parseHeadcountValue(%q) = (%d, %v), want (0, false)", i, "one or two", value, ok)
		}
	}
}

func TestHeadcountNeedsReview(t *testing.T) {
	if headcountNeedsReview(3) {
		t.Error("3 must not need review")
	}
	if headcountNeedsReview(20) {
		t.Error("20 is the ceiling and must not need review")
	}
	if !headcountNeedsReview(21) {
		t.Error("21 must need review")
	}
	if !headcountNeedsReview(999) {
		t.Error("999 must need review")
	}
}
