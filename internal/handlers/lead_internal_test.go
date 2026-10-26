package handlers

import (
	"testing"

	"github.com/nikyjain/whatomate/internal/models"
)

func TestNormalizeLeadAPIVersion(t *testing.T) {
	cases := map[string]struct {
		want  string
		valid bool
	}{
		"":               {defaultLeadAPIVersion, true},
		"v21.0":          {"v21.0", true},
		" v19.0 ":        {"v19.0", true},
		"v21":            {"v21", false},
		"v21.0/../me":    {"v21.0/../me", false},
		"21.0":           {"21.0", false},
		"v21.0?fields=x": {"v21.0?fields=x", false},
	}
	for in, tc := range cases {
		got, valid := normalizeLeadAPIVersion(in)
		if valid != tc.valid || (valid && got != tc.want) {
			t.Errorf("normalizeLeadAPIVersion(%q) = %q,%v want %q,%v", in, got, valid, tc.want, tc.valid)
		}
	}
}

func TestMergeContactTags(t *testing.T) {
	existing := models.JSONBArray{"vip", "lead"}
	merged, changed := mergeContactTags(existing, models.StringArray{"lead", "fb-ads", ""})
	if !changed {
		t.Fatal("expected change when a new tag is added")
	}
	if len(merged) != 3 || merged[2] != "fb-ads" {
		t.Fatalf("unexpected merge result %v", merged)
	}
	if _, changed := mergeContactTags(merged, models.StringArray{"vip"}); changed {
		t.Fatal("no change expected when all tags already present")
	}
}

func TestClaimableStatusesExcludeSending(t *testing.T) {
	for _, s := range claimableLeadStatuses {
		if s == models.StatusLeadSending || s == models.StatusLeadSent {
			t.Fatalf("status %q must never be auto-claimed", s)
		}
	}
}
