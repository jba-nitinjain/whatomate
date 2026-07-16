package handlers

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/config"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/valyala/fasthttp"
)

func TestRSVPReminderCampaignOutcomeAllFailed(t *testing.T) {
	cases := []struct {
		name       string
		sent, fail int
		total      int
		want       string
	}{
		{"all failed", 0, 1008, 1008, "failed"},
		{"partial", 900, 108, 1008, "completed_with_errors"},
		{"clean", 1008, 0, 1008, "completed"},
		{"empty", 0, 0, 0, "completed"},
	}
	for _, c := range cases {
		if got := rsvpReminderCampaignOutcome(c.sent, c.fail, c.total); got != c.want {
			t.Errorf("%s: rsvpReminderCampaignOutcome(%d,%d,%d) = %q, want %q",
				c.name, c.sent, c.fail, c.total, got, c.want)
		}
	}
}

// TestRSVPReminderMediaValidationError_RejectsMissingStagingID pins the fix for
// the 15/07/2026 incident: SendRSVPReminders (rsvp_reminders.go) now calls
// this function before createRSVPReminderCampaign so a VIDEO/IMAGE/DOCUMENT
// header template with no staged file is rejected up front with a clean,
// user-fixable 400 - instead of reaching createRSVPReminderCampaign and
// failing 1008 times against Meta with error 132012, or (post-fix, without
// this pre-check) leaking createRSVPReminderCampaign's raw infrastructure
// errors to the client. Without this check the handler would fall through to
// createRSVPReminderCampaign's own validateCampaignReadyForStart gate deep
// inside a transaction-adjacent path, which this test would not catch failing.
func TestRSVPReminderMediaValidationError_RejectsMissingStagingID(t *testing.T) {
	cases := []struct {
		name       string
		headerType string
		stagingID  string
		wantErr    string
	}{
		{"video no staging id", "VIDEO", "", "template requires video header media. Configure campaign media before starting"},
		{"image no staging id", "IMAGE", "", "template requires image header media. Configure campaign media before starting"},
		{"document no staging id", "DOCUMENT", "", "template requires document header media. Configure campaign media before starting"},
		{"video with staging id ok", "VIDEO", "abc123", ""},
		{"whitespace staging id rejected", "VIDEO", "   ", "template requires video header media. Configure campaign media before starting"},
		{"text header needs nothing", "TEXT", "", ""},
		{"empty header type needs nothing", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			template := &models.Template{HeaderType: c.headerType}
			err := rsvpReminderMediaValidationError(template, c.stagingID)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("rsvpReminderMediaValidationError(%q, %q) = %v, want nil", c.headerType, c.stagingID, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("rsvpReminderMediaValidationError(%q, %q) = nil, want error %q", c.headerType, c.stagingID, c.wantErr)
			}
			if err.Error() != c.wantErr {
				t.Fatalf("rsvpReminderMediaValidationError(%q, %q) = %q, want %q", c.headerType, c.stagingID, err.Error(), c.wantErr)
			}
		})
	}
}

// TestRSVPReminderCampaignErrorEnvelope pins the fix for the gap left by the
// generic-500 fix above: createRSVPReminderCampaign can also fail with a
// rsvpUserFacingError (missing/expired staged media, or the
// validateCampaignReadyForStart backstop) that must reach the client as a 400
// with its own message - not the same opaque 500 as a real infrastructure
// failure (a DB write or the campaign queue being unavailable). This asserts
// rsvpReminderCampaignErrorEnvelope tells the two apart via errors.As, and
// that the distinction survives an extra layer of %w wrapping.
func TestRSVPReminderCampaignErrorEnvelope(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "user-facing error surfaces its own message as a 400",
			err:        rsvpUserFacingError{fmt.Errorf("staged media not found - it may have expired or already been used")},
			wantStatus: fasthttp.StatusBadRequest,
			wantMsg:    "staged media not found - it may have expired or already been used",
		},
		{
			name:       "user-facing error wrapped further by %w is still detected",
			err:        fmt.Errorf("promote staged media: %w", rsvpUserFacingError{fmt.Errorf("failed to read staged media: file removed")}),
			wantStatus: fasthttp.StatusBadRequest,
			wantMsg:    "failed to read staged media: file removed",
		},
		{
			name:       "plain infrastructure error stays a generic 500, not leaked",
			err:        fmt.Errorf("campaign queue is unavailable"),
			wantStatus: fasthttp.StatusInternalServerError,
			wantMsg:    "Failed to create reminder campaign",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, msg := rsvpReminderCampaignErrorEnvelope(c.err)
			if status != c.wantStatus || msg != c.wantMsg {
				t.Fatalf("rsvpReminderCampaignErrorEnvelope(%v) = (%d, %q), want (%d, %q)", c.err, status, msg, c.wantStatus, c.wantMsg)
			}
		})
	}
}

// TestLoadStagedRSVPReminderMedia_ExpiredFileIsUserFacing is the test the task
// asked for: an expired/missing staged file must yield the user-facing
// message, not the generic "Failed to create reminder campaign" 500. It needs
// no database - loadStagedRSVPReminderMedia only touches Config.Storage.LocalPath
// and the filesystem - so it exercises the real production call chain
// (loadStagedRSVPReminderMedia -> rsvpReminderCampaignErrorEnvelope) end to
// end, not a synthetic error. Without the Finding 1 fix (either the wrapping
// in rsvp_reminder_media.go or the errors.As check in
// rsvp_reminders.go/rsvp_reminder_campaign_test.go) this fails: the
// errors.As assertion fails if the wrapping is missing, and the status
// assertion fails (500 instead of 400) if the classification is missing.
func TestLoadStagedRSVPReminderMedia_ExpiredFileIsUserFacing(t *testing.T) {
	app := &App{Config: &config.Config{Storage: config.StorageConfig{LocalPath: t.TempDir()}}}
	// A syntactically valid staging id for which UploadRSVPReminderMedia never
	// wrote a file - the same shape as a real expired/cleaned-up upload.
	stagingID := uuid.New().String()

	_, _, err := app.loadStagedRSVPReminderMedia(stagingID)
	if err == nil {
		t.Fatal("loadStagedRSVPReminderMedia() = nil error, want an error for a staging id with no staged file")
	}

	var userErr rsvpUserFacingError
	if !errors.As(err, &userErr) {
		t.Fatalf("loadStagedRSVPReminderMedia(%q) = %v, want a rsvpUserFacingError so SendRSVPReminders can surface it as a 400 instead of a generic 500", stagingID, err)
	}

	status, msg := rsvpReminderCampaignErrorEnvelope(err)
	if status != fasthttp.StatusBadRequest {
		t.Fatalf("rsvpReminderCampaignErrorEnvelope(%v) status = %d, want %d (an expired/missing staged file must not fall back to the generic 500)", err, status, fasthttp.StatusBadRequest)
	}
	wantMsg := "staged media not found - it may have expired or already been used"
	if msg != wantMsg {
		t.Fatalf("rsvpReminderCampaignErrorEnvelope(%v) message = %q, want %q", err, msg, wantMsg)
	}
}
