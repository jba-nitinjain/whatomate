package handlers

import (
	"strings"
	"testing"

	"github.com/nikyjain/whatomate/internal/config"
	"github.com/nikyjain/whatomate/test/testutil"
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

// TestLoadStagedRSVPReminderMedia_DerivesMimeTypeFromDisk pins the fix for the
// bug found in review: the send request used to carry a client-echoed
// "staging_mime_type" that was used to rebuild the staged file's path
// (rsvp_reminder_campaign.go, before this change). A mismatched echo pointed
// HeaderMediaLocalPath at a file that does not exist. loadStagedRSVPReminderMedia
// takes no MIME type input at all - it locates the staged file on disk and
// derives its type from the file itself.
func TestLoadStagedRSVPReminderMedia_DerivesMimeTypeFromDisk(t *testing.T) {
	app := &App{Log: testutil.NopLogger(), Config: &config.Config{Storage: config.StorageConfig{LocalPath: t.TempDir()}}}

	stagingID := "abc123"
	key := rsvpReminderStagingKey(stagingID)
	pdfBytes := []byte("%PDF-1.4 fake reminder attachment")
	if _, err := app.saveCampaignMedia(key, pdfBytes, "application/pdf"); err != nil {
		t.Fatalf("saveCampaignMedia: %v", err)
	}

	data, mimeType, err := app.loadStagedRSVPReminderMedia(stagingID)
	if err != nil {
		t.Fatalf("loadStagedRSVPReminderMedia: %v", err)
	}
	if string(data) != string(pdfBytes) {
		t.Fatalf("loaded data = %q, want %q", data, pdfBytes)
	}
	if mimeType != "application/pdf" {
		t.Fatalf("mimeType = %q, want application/pdf (derived from disk, not a client hint)", mimeType)
	}
}

func TestLoadStagedRSVPReminderMedia_MissingFileErrors(t *testing.T) {
	app := &App{Log: testutil.NopLogger(), Config: &config.Config{Storage: config.StorageConfig{LocalPath: t.TempDir()}}}

	if _, _, err := app.loadStagedRSVPReminderMedia("never-staged"); err == nil {
		t.Fatal("expected an error for a staging id with no file on disk")
	}
}

func TestLoadStagedRSVPReminderMedia_RejectsUnsafeStagingID(t *testing.T) {
	app := &App{Log: testutil.NopLogger(), Config: &config.Config{Storage: config.StorageConfig{LocalPath: t.TempDir()}}}

	if _, _, err := app.loadStagedRSVPReminderMedia("../secrets"); err == nil {
		t.Fatal("expected an error for a staging id that fails stagingIDPattern")
	}
}
