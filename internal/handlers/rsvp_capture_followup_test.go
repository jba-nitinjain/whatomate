package handlers

import (
	"testing"

	"github.com/nikyjain/whatomate/internal/models"
)

func TestMergeRSVPAnswersKeepsExisting(t *testing.T) {
	// The live event has 271 members who already answered attendance and spouse.
	// A follow-up asking only about children must not erase any of it.
	existing := models.JSONB{
		"attendance":              "yes",
		"attendance_title":        "Attending",
		"spouse_attendance":       "yes",
		"spouse_attendance_title": "Attending",
		"spouse_mobile":           "919840026019",
	}
	incoming := models.JSONB{
		"children_count": "2",
	}

	got := mergeRSVPAnswers(existing, incoming)

	for k, want := range existing {
		if got[k] != want {
			t.Errorf("follow-up erased %q: got %v, want %v", k, got[k], want)
		}
	}
	if got["children_count"] != "2" {
		t.Errorf("children_count = %v, want 2", got["children_count"])
	}
	if len(got) != 6 {
		t.Errorf("expected 6 keys, got %d: %+v", len(got), got)
	}
}

func TestMergeRSVPAnswersIncomingWins(t *testing.T) {
	// A guest correcting an earlier answer must be honoured.
	existing := models.JSONB{"children_count": "2"}
	incoming := models.JSONB{"children_count": "3"}

	got := mergeRSVPAnswers(existing, incoming)
	if got["children_count"] != "3" {
		t.Errorf("incoming must win: got %v, want 3", got["children_count"])
	}
}

func TestMergeRSVPAnswersHandlesNil(t *testing.T) {
	got := mergeRSVPAnswers(nil, models.JSONB{"a": "1"})
	if got["a"] != "1" {
		t.Errorf("nil existing must yield incoming: %+v", got)
	}

	got = mergeRSVPAnswers(models.JSONB{"a": "1"}, nil)
	if got["a"] != "1" {
		t.Errorf("nil incoming must preserve existing: %+v", got)
	}

	if got := mergeRSVPAnswers(nil, nil); len(got) != 0 {
		t.Errorf("nil/nil must yield empty, got %+v", got)
	}
}

func TestMergeRSVPAnswersDoesNotAliasExisting(t *testing.T) {
	existing := models.JSONB{"a": "1"}
	incoming := models.JSONB{"b": "2"}

	got := mergeRSVPAnswers(existing, incoming)
	got["c"] = "3"

	if _, leaked := existing["c"]; leaked {
		t.Error("merge must not mutate the existing map")
	}
}
