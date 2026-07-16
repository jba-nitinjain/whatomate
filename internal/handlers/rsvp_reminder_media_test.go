package handlers

import (
	"strings"
	"testing"
)

func TestRSVPReminderStagingKeyIsScoped(t *testing.T) {
	// The value is passed to saveCampaignMedia as its campaignID, which writes to
	// <media>/campaigns/<value><ext>. It must stay a flat filename: ensureMediaDir
	// only creates "campaigns", so any subdirectory would fail to write.
	key := rsvpReminderStagingKey("abc123")
	if !strings.HasPrefix(key, "staging-") {
		t.Fatalf("staging key must be a flat staging- filename, got %q", key)
	}
	if !strings.Contains(key, "abc123") {
		t.Fatalf("staging key must contain the staging id, got %q", key)
	}
	if strings.ContainsAny(key, `/\`) {
		t.Fatalf("staging key must contain no path separator, got %q", key)
	}
}

func TestRSVPReminderStagingKeyRejectsTraversal(t *testing.T) {
	// staging_id arrives from the client and is used to build a filesystem path.
	for _, bad := range []string{"../secrets", "a/b", "..", "a\\b", "", "a.b"} {
		if got := rsvpReminderStagingKey(bad); got != "" {
			t.Errorf("rsvpReminderStagingKey(%q) = %q, want \"\" (rejected)", bad, got)
		}
	}
}
