package handlers

import (
	"testing"

	"github.com/nikyjain/whatomate/internal/models"
)

func TestValidateCampaignReadyForStart_AcceptsLocalPathOnly(t *testing.T) {
	// worker.go:144-145 sends HeaderMediaLocalPath, so a campaign carrying only
	// a local path is sendable and must not be rejected.
	app := &App{}
	campaign := &models.BulkMessageCampaign{
		Template:             &models.Template{HeaderType: "VIDEO"},
		HeaderMediaLocalPath: "campaigns/abc.mp4",
	}

	if err := app.validateCampaignReadyForStart(campaign); err != nil {
		t.Fatalf("expected local path to satisfy media header, got: %v", err)
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
