package handlers

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/utils"
	"github.com/valyala/fasthttp"
	"github.com/xuri/excelize/v2"
	"github.com/zerodha/fastglue"
)

type campaignReportData struct {
	Campaign     models.BulkMessageCampaign
	Recipients   []models.BulkMessageRecipient
	Timezone     string
	Location     *time.Location
	GeneratedAt  time.Time
}

var campaignReportFilenameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// ExportCampaignReport returns a formatted XLSX report for a campaign.
func (a *App) ExportCampaignReport(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	id, err := parsePathUUID(r, "id", "campaign")
	if err != nil {
		return nil
	}

	var campaign models.BulkMessageCampaign
	if err := a.DB.Where("id = ? AND organization_id = ?", id, orgID).
		Preload("Template").
		First(&campaign).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Campaign not found", nil, "")
	}

	var org models.Organization
	if err := a.DB.Select("settings").Where("id = ?", orgID).First(&org).Error; err != nil {
		a.Log.Error("Failed to load organization settings for campaign report", "error", err, "campaign_id", id)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to generate campaign report", nil, "")
	}

	var recipients []models.BulkMessageRecipient
	if err := a.DB.Where("campaign_id = ?", campaign.ID).
		Order("created_at ASC").
		Find(&recipients).Error; err != nil {
		a.Log.Error("Failed to load campaign recipients for report", "error", err, "campaign_id", id)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to generate campaign report", nil, "")
	}

	if a.ShouldMaskPhoneNumbers(orgID) {
		for i := range recipients {
			recipients[i].PhoneNumber = utils.MaskPhoneNumber(recipients[i].PhoneNumber)
			recipients[i].RecipientName = utils.MaskIfPhoneNumber(recipients[i].RecipientName)
		}
	}

	reportTimezone, reportLocation := campaignReportTimezone(org.Settings)

	file, err := buildCampaignReportWorkbook(campaignReportData{
		Campaign:    campaign,
		Recipients:  recipients,
		Timezone:    reportTimezone,
		Location:    reportLocation,
		GeneratedAt: time.Now(),
	})
	if err != nil {
		a.Log.Error("Failed to build campaign report workbook", "error", err, "campaign_id", id)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to generate campaign report", nil, "")
	}
	defer func() {
		_ = file.Close()
	}()

	buf, err := file.WriteToBuffer()
	if err != nil {
		a.Log.Error("Failed to serialize campaign report workbook", "error", err, "campaign_id", id)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to generate campaign report", nil, "")
	}

	filename := campaignReportFilename(campaign.Name, time.Now())
	r.RequestCtx.Response.Header.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	r.RequestCtx.Response.Header.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	r.RequestCtx.SetBody(buf.Bytes())
	return nil
}

func campaignReportTimezone(settings models.JSONB) (string, *time.Location) {
	timezone := "UTC"
	if settings != nil {
		if v, ok := settings["timezone"].(string); ok && strings.TrimSpace(v) != "" {
			timezone = strings.TrimSpace(v)
		}
	}

	location, err := time.LoadLocation(timezone)
	if err != nil {
		return "UTC", time.UTC
	}
	return timezone, location
}

func campaignReportFilename(name string, now time.Time) string {
	cleanName := strings.TrimSpace(strings.ToLower(name))
	cleanName = campaignReportFilenameSanitizer.ReplaceAllString(cleanName, "_")
	cleanName = strings.Trim(cleanName, "_")
	if cleanName == "" {
		cleanName = "campaign_report"
	}
	return fmt.Sprintf("%s_%s.xlsx", cleanName, now.Format("20060102_150405"))
}

func mustNewReportStyle(file *excelize.File, style *excelize.Style) int {
	id, err := file.NewStyle(style)
	if err != nil {
		panic(err)
	}
	return id
}
