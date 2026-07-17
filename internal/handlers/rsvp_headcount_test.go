package handlers

import (
	"testing"

	"github.com/nikyjain/whatomate/internal/models"
)

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

func TestEvaluateHeadcountContributorBoolean(t *testing.T) {
	c := models.RSVPHeadcountContributor{
		Label: "Spouse", AnswerKey: "spouse_attendance",
		Mode: models.RSVPHeadcountModeBoolean, MatchValues: []string{"yes", "attending"},
	}

	got := evaluateHeadcountContributor(c, models.JSONB{"spouse_attendance": "yes"}, models.RSVPAttendanceYes)
	if got.People != 1 || !got.Matched {
		t.Fatalf("expected 1 person matched, got %+v", got)
	}

	// The _title companion must satisfy the same contributor.
	got = evaluateHeadcountContributor(c, models.JSONB{"spouse_attendance_title": "Attending"}, models.RSVPAttendanceYes)
	if got.People != 1 || !got.Matched {
		t.Fatalf("_title companion must match, got %+v", got)
	}

	got = evaluateHeadcountContributor(c, models.JSONB{"spouse_attendance": "no"}, models.RSVPAttendanceYes)
	if got.People != 0 || got.Matched {
		t.Fatalf("expected no match, got %+v", got)
	}

	got = evaluateHeadcountContributor(c, models.JSONB{}, models.RSVPAttendanceYes)
	if got.People != 0 || got.Matched {
		t.Fatalf("absent answer must not match, got %+v", got)
	}
}

func TestEvaluateHeadcountContributorBooleanIsCaseInsensitive(t *testing.T) {
	c := models.RSVPHeadcountContributor{
		AnswerKey: "spouse_attendance", Mode: models.RSVPHeadcountModeBoolean,
		MatchValues: []string{"Yes", "ATTENDING"},
	}
	got := evaluateHeadcountContributor(c, models.JSONB{"spouse_attendance": "  yes  "}, models.RSVPAttendanceYes)
	if got.People != 1 {
		t.Fatalf("matching must be case- and space-insensitive, got %+v", got)
	}
}

func TestEvaluateHeadcountContributorNumeric(t *testing.T) {
	c := models.RSVPHeadcountContributor{
		Label: "Children", AnswerKey: "children_count", Mode: models.RSVPHeadcountModeNumeric,
	}

	got := evaluateHeadcountContributor(c, models.JSONB{"children_count": "3"}, models.RSVPAttendanceYes)
	if got.People != 3 || !got.Matched {
		t.Fatalf("expected 3, got %+v", got)
	}

	got = evaluateHeadcountContributor(c, models.JSONB{"children_count": "abc"}, models.RSVPAttendanceYes)
	if got.People != 0 || !got.Unparseable {
		t.Fatalf("unparseable must count 0 and flag, got %+v", got)
	}

	got = evaluateHeadcountContributor(c, models.JSONB{"children_count": "50"}, models.RSVPAttendanceYes)
	if got.People != 50 || !got.NeedsReview {
		t.Fatalf("50 must count but flag for review, got %+v", got)
	}

	got = evaluateHeadcountContributor(c, models.JSONB{}, models.RSVPAttendanceYes)
	if got.People != 0 || got.Unparseable {
		t.Fatalf("absent answer is 0 and not a parse failure, got %+v", got)
	}
}

// TestEvaluateHeadcountContributorNumeric_ZeroIsMatched guards the fix for
// Matched conflating "answered zero" with "didn't answer": a guest who
// validly answers "0 children" must still show up as a matched response
// (People: 0), distinct from a guest who never answered at all.
func TestEvaluateHeadcountContributorNumeric_ZeroIsMatched(t *testing.T) {
	c := models.RSVPHeadcountContributor{
		Label: "Children", AnswerKey: "children_count", Mode: models.RSVPHeadcountModeNumeric,
	}

	got := evaluateHeadcountContributor(c, models.JSONB{"children_count": "0"}, models.RSVPAttendanceYes)
	if got.People != 0 || !got.Matched || got.Unparseable {
		t.Fatalf("a valid zero answer must be Matched with People 0, got %+v", got)
	}

	got = evaluateHeadcountContributor(c, models.JSONB{}, models.RSVPAttendanceYes)
	if got.Matched {
		t.Fatalf("an absent answer must not be Matched, got %+v", got)
	}
}

func TestEvaluateHeadcountContributorAttendance(t *testing.T) {
	c := models.RSVPHeadcountContributor{
		Label: "Member attendance", Mode: models.RSVPHeadcountModeAttendance,
		MatchValues: []string{string(models.RSVPAttendanceYes)},
	}

	got := evaluateHeadcountContributor(c, models.JSONB{}, models.RSVPAttendanceYes)
	if got.People != 1 || !got.Matched {
		t.Fatalf("attendance mode must read the attendance column even with empty answers, got %+v", got)
	}

	got = evaluateHeadcountContributor(c, models.JSONB{}, models.RSVPAttendanceNo)
	if got.People != 0 || got.Matched {
		t.Fatalf("non-yes attendance column must not match, got %+v", got)
	}

	// AnswerKey is unused in attendance mode: a raw code sitting in the
	// answers JSONB under "attendance" must not influence the result at all.
	got = evaluateHeadcountContributor(c, models.JSONB{"attendance": "btn_42"}, models.RSVPAttendanceYes)
	if got.People != 1 || !got.Matched {
		t.Fatalf("attendance mode must ignore the answers JSONB, got %+v", got)
	}
}

func TestLegacyHeadcountContributors(t *testing.T) {
	// Events predating this feature must tally exactly as before.
	got := legacyHeadcountContributors("attendance")
	if len(got) != 2 {
		t.Fatalf("expected member + spouse, got %d: %+v", len(got), got)
	}
	if got[0].Mode != models.RSVPHeadcountModeAttendance {
		t.Fatalf("first must be member attendance, read from the attendance column: %+v", got[0])
	}
	if len(got[0].MatchValues) != 1 || got[0].MatchValues[0] != string(models.RSVPAttendanceYes) {
		t.Fatalf("member attendance must match against the column's own \"yes\" vocabulary: %+v", got[0])
	}
	if got[1].AnswerKey != "spouse_attendance" || got[1].Mode != models.RSVPHeadcountModeBoolean {
		t.Fatalf("second must be spouse attendance: %+v", got[1])
	}
}

// TestLegacyHeadcountContributors_MatchesLiveAttendanceColumn is the live-data
// regression this fix addresses. UpdateRSVPResponse lets an admin PATCH
// resp.Attendance without touching resp.Answers, and a non-identity
// AttendanceMap can map a raw button code into the attendance column while
// leaving a different raw value in the answers JSONB. Either way, the member
// contributor must agree with buildRSVPAttendanceBreakdown (rsvp_tally.go),
// which counts strictly from the response.Attendance column. Reverting the
// member contributor to AnswerKey/boolean mode makes both of these fail,
// because normalizedRSVPAnswer reads the (here absent or misleading) answers
// JSONB instead of the attendance column.
func TestLegacyHeadcountContributors_MatchesLiveAttendanceColumn(t *testing.T) {
	member := legacyHeadcountContributors("attendance")[0]

	// Admin manually set Attendance=yes via PATCH; Answers was never touched.
	got := evaluateHeadcountContributor(member, models.JSONB{}, models.RSVPAttendanceYes)
	if got.People != 1 || !got.Matched {
		t.Fatalf("a response with Attendance=yes and no answers must count 1 member, got %+v", got)
	}

	// AttendanceMap mapped a raw button code to "yes" in the column, but the
	// raw code (not "yes"/"attending") is what's stored in the answers JSONB.
	got = evaluateHeadcountContributor(member, models.JSONB{"attendance": "btn_rsvp_1"}, models.RSVPAttendanceYes)
	if got.People != 1 || !got.Matched {
		t.Fatalf("a yes attendance column must count 1 member regardless of the raw answers value, got %+v", got)
	}
}
