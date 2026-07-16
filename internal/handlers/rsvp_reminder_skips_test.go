package handlers

import (
	"testing"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
)

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

// TestRSVPReminderEligibilityDedupesDuplicatePhones would fail if preview and
// send disagreed on duplicates: before this fix, RSVPReminderPreview never
// called dedupeRSVPReminderRows, so two eligible rows sharing a normalized
// phone were both counted as eligible in preview while send (which does
// dedupe via createRSVPReminderCampaign) would only queue one and record the
// other as skipped. rsvpReminderEligibility is the exact predicate+dedupe
// preview now runs, mirroring dedupeRSVPReminderRowsWithSkips used by send.
func TestRSVPReminderEligibilityDedupesDuplicatePhones(t *testing.T) {
	contact := &models.Contact{ProfileName: "Priya"}
	rows := []models.RSVPResponse{
		{BaseModel: models.BaseModel{ID: uuid.New()}, PhoneNumber: "9840445616", Contact: contact},
		{BaseModel: models.BaseModel{ID: uuid.New()}, PhoneNumber: "919840445616", Contact: contact}, // same person, different format
		{BaseModel: models.BaseModel{ID: uuid.New()}, PhoneNumber: "919999999999", Contact: contact},
	}

	eligible, skipped := rsvpReminderEligibility(rows)

	if eligible != 2 {
		t.Fatalf("expected 2 eligible (one row deduped as a duplicate phone), got %d", eligible)
	}
	if len(skipped) != 1 {
		t.Fatalf("expected exactly 1 skipped duplicate, got %d: %+v", len(skipped), skipped)
	}
	if skipped[0].Reason != "duplicate phone number" {
		t.Fatalf("expected duplicate to be reported with reason %q, got %q", "duplicate phone number", skipped[0].Reason)
	}
	if skipped[0].ResponseID != rows[1].ID {
		t.Fatalf("expected the second (later) row to be the one dropped, first-wins; got response_id %v want %v", skipped[0].ResponseID, rows[1].ID)
	}
}

// TestRSVPReminderEligibilityMatchesSendDedupe pins that preview's dedupe
// (rsvpReminderEligibility) and send's dedupe (dedupeRSVPReminderRowsWithSkips,
// called from createRSVPReminderCampaign) are the same operation, so the two
// code paths cannot drift back apart silently.
func TestRSVPReminderEligibilityMatchesSendDedupe(t *testing.T) {
	contact := &models.Contact{ProfileName: "Priya"}
	rows := []models.RSVPResponse{
		{BaseModel: models.BaseModel{ID: uuid.New()}, PhoneNumber: "9840445616", Contact: contact},
		{BaseModel: models.BaseModel{ID: uuid.New()}, PhoneNumber: "919840445616", Contact: contact},
	}

	previewEligible, previewSkipped := rsvpReminderEligibility(rows)
	sendKept, sendSkipped := dedupeRSVPReminderRowsWithSkips(rows)

	if previewEligible != len(sendKept) {
		t.Fatalf("preview eligible (%d) must equal send's kept-row count (%d)", previewEligible, len(sendKept))
	}
	if len(previewSkipped) != len(sendSkipped) {
		t.Fatalf("preview skipped (%d) must equal send's skipped duplicate count (%d)", len(previewSkipped), len(sendSkipped))
	}
}
