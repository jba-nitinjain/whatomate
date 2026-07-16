package handlers

import (
	"strings"
	"testing"

	"github.com/nikyjain/whatomate/internal/config"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
)

func TestValidateCampaignReadyForStart_RejectsLocalPathOnly(t *testing.T) {
	// HeaderMediaLocalPath is for local chat rendering (worker.go:144-147); the send
	// uses HeaderMediaID/HeaderMediaURL (worker.go:122). A campaign carrying only a
	// local path sends no header component and Meta rejects it with 132012, so it
	// must NOT pass validation.
	app := &App{}
	campaign := &models.BulkMessageCampaign{
		Template:             &models.Template{HeaderType: "VIDEO"},
		HeaderMediaLocalPath: "campaigns/abc.mp4",
	}
	if err := app.validateCampaignReadyForStart(campaign); err == nil {
		t.Fatal("a local path alone must not satisfy a media header - nothing would be sent")
	}
}

func TestValidateCampaignReadyForStart_RejectsWhenAllMediaFieldsEmpty(t *testing.T) {
	app := &App{}
	campaign := &models.BulkMessageCampaign{
		Template: &models.Template{HeaderType: "VIDEO"},
	}

	err := app.validateCampaignReadyForStart(campaign)
	if err == nil {
		t.Fatal("expected rejection when no media is present")
	}
	if got, want := err.Error(), "template requires video header media. Configure campaign media before starting"; got != want {
		t.Fatalf("error text changed:\n got: %q\nwant: %q", got, want)
	}
}

func TestValidateCampaignReadyForStart_AcceptsIDOrURL(t *testing.T) {
	app := &App{}

	byID := &models.BulkMessageCampaign{
		Template:      &models.Template{HeaderType: "IMAGE"},
		HeaderMediaID: "media-123",
	}
	if err := app.validateCampaignReadyForStart(byID); err != nil {
		t.Fatalf("HeaderMediaID must still satisfy: %v", err)
	}

	byURL := &models.BulkMessageCampaign{
		Template:       &models.Template{HeaderType: "DOCUMENT"},
		HeaderMediaURL: "https://example.test/f.pdf",
	}
	if err := app.validateCampaignReadyForStart(byURL); err != nil {
		t.Fatalf("HeaderMediaURL must still satisfy: %v", err)
	}
}

func TestValidateCampaignReadyForStart_TextHeaderNeedsNoMedia(t *testing.T) {
	app := &App{}
	campaign := &models.BulkMessageCampaign{
		Template: &models.Template{HeaderType: "TEXT"},
	}
	if err := app.validateCampaignReadyForStart(campaign); err != nil {
		t.Fatalf("TEXT header must not require media: %v", err)
	}
}

func TestValidateCampaignReadyForStart_WhitespaceOnlyLocalPathIsNotMedia(t *testing.T) {
	app := &App{}
	campaign := &models.BulkMessageCampaign{
		Template:             &models.Template{HeaderType: "VIDEO"},
		HeaderMediaLocalPath: "   ",
	}
	if err := app.validateCampaignReadyForStart(campaign); err == nil {
		t.Fatal("whitespace-only local path must not count as media")
	}
}

// TestPublicBaseURLFromConfig_UsesConfiguredURL covers the scheduler path
// (rsvp_scheduler.go), which has no HTTP request to derive a base URL from -
// it must use server.public_url instead.
func TestPublicBaseURLFromConfig_UsesConfiguredURL(t *testing.T) {
	app := &App{Log: testutil.NopLogger(), Config: &config.Config{
		Server: config.ServerConfig{PublicURL: "https://reminders.example.com/", BasePath: "/whatomate"},
	}}
	got := app.publicBaseURLFromConfig()
	want := "https://reminders.example.com/whatomate"
	if got != want {
		t.Fatalf("publicBaseURLFromConfig() = %q, want %q", got, want)
	}
}

// TestPublicBaseURLFromConfig_FallsBackWhenUnconfigured pins "never silently
// URL-less": even with server.public_url unset, a scheduled reminder must
// still get a non-empty media URL rather than the empty string the RSVP path
// sent before this fix (the exact shape of the 15/07/2026 failure).
func TestPublicBaseURLFromConfig_FallsBackWhenUnconfigured(t *testing.T) {
	app := &App{Log: testutil.NopLogger(), Config: &config.Config{
		Server: config.ServerConfig{Host: "0.0.0.0", Port: 8080},
	}}
	got := app.publicBaseURLFromConfig()
	if got == "" {
		t.Fatal("publicBaseURLFromConfig() must never return an empty string")
	}
	if !strings.Contains(got, "8080") {
		t.Fatalf("publicBaseURLFromConfig() = %q, want it to include the configured port", got)
	}
}
