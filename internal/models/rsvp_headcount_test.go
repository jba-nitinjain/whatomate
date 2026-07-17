package models

import (
	"testing"
)

func TestRSVPHeadcountContributorsRoundTrip(t *testing.T) {
	original := RSVPHeadcountContributors{
		{Label: "Member attendance", AnswerKey: "attendance", Mode: RSVPHeadcountModeBoolean, MatchValues: []string{"yes", "attending"}},
		{Label: "Children", AnswerKey: "children_count", Mode: RSVPHeadcountModeNumeric},
	}

	value, err := original.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}

	var restored RSVPHeadcountContributors
	if err := restored.Scan(value); err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if len(restored) != 2 {
		t.Fatalf("expected 2 contributors, got %d", len(restored))
	}
	if restored[0].Label != "Member attendance" || restored[0].Mode != RSVPHeadcountModeBoolean {
		t.Fatalf("first contributor corrupted: %+v", restored[0])
	}
	if len(restored[0].MatchValues) != 2 || restored[0].MatchValues[0] != "yes" {
		t.Fatalf("match values corrupted: %+v", restored[0].MatchValues)
	}
	if restored[1].AnswerKey != "children_count" || restored[1].Mode != RSVPHeadcountModeNumeric {
		t.Fatalf("second contributor corrupted: %+v", restored[1])
	}
}

func TestRSVPHeadcountContributorsScanNil(t *testing.T) {
	// A row written before this column existed scans as NULL and must yield an
	// empty list, not an error - the live event predates this feature.
	var c RSVPHeadcountContributors
	if err := c.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) must not error: %v", err)
	}
	if len(c) != 0 {
		t.Fatalf("Scan(nil) must yield empty, got %+v", c)
	}
}

func TestRSVPHeadcountContributorsScanEmptyArray(t *testing.T) {
	var c RSVPHeadcountContributors
	if err := c.Scan([]byte(`[]`)); err != nil {
		t.Fatalf("Scan([]) error: %v", err)
	}
	if len(c) != 0 {
		t.Fatalf("expected empty, got %+v", c)
	}
}

func TestRSVPHeadcountContributorsScanGarbage(t *testing.T) {
	var c RSVPHeadcountContributors
	if err := c.Scan([]byte(`{"not":"an array"}`)); err == nil {
		t.Fatal("expected an error scanning a non-array")
	}
}

func TestRSVPHeadcountContributorsValueEmptyIsArrayNotNull(t *testing.T) {
	// Must serialize to [] so the jsonb column default and the API shape agree;
	// a null would make the frontend guard for two empty representations.
	var c RSVPHeadcountContributors
	v, err := c.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	if string(v.([]byte)) != "[]" {
		t.Fatalf("empty contributors must serialize to [], got %s", v.([]byte))
	}
}
