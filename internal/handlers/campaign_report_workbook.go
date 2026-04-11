package handlers

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/xuri/excelize/v2"
)

const (
	campaignReportSummarySheet    = "Summary"
	campaignReportRecipientsSheet = "Recipients"
	campaignReportTimeLayout      = "2006-01-02 15:04:05"
)

func buildCampaignReportWorkbook(report campaignReportData) (*excelize.File, error) {
	file := excelize.NewFile()
	file.SetSheetName(file.GetSheetName(0), campaignReportSummarySheet)
	file.NewSheet(campaignReportRecipientsSheet)

	titleStyle, err := file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 16},
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	if err != nil {
		return nil, err
	}
	labelStyle, err := file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#F3F4F6"}, Pattern: 1},
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	if err != nil {
		return nil, err
	}
	headerStyle, err := file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#1F4E78"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return nil, err
	}

	statusStyles := make(map[models.MessageStatus]int)
	for status, color := range map[models.MessageStatus]string{
		models.MessageStatusPending:   "#FEF3C7",
		models.MessageStatusSent:      "#DBEAFE",
		models.MessageStatusDelivered: "#DCFCE7",
		models.MessageStatusRead:      "#D1FAE5",
		models.MessageStatusFailed:    "#FEE2E2",
	} {
		styleID, styleErr := file.NewStyle(&excelize.Style{
			Fill:      excelize.Fill{Type: "pattern", Color: []string{color}, Pattern: 1},
			Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		})
		if styleErr != nil {
			return nil, styleErr
		}
		statusStyles[status] = styleID
	}

	if err := populateCampaignSummarySheet(file, report, titleStyle, labelStyle); err != nil {
		return nil, err
	}
	if err := populateCampaignRecipientsSheet(file, report, headerStyle, statusStyles); err != nil {
		return nil, err
	}

	file.SetActiveSheet(0)
	return file, nil
}

func populateCampaignSummarySheet(file *excelize.File, report campaignReportData, titleStyle, labelStyle int) error {
	campaign := report.Campaign
	location := campaignReportLocation(report.Location)
	templateName := ""
	templateLanguage := ""
	if campaign.Template != nil {
		templateName = campaign.Template.Name
		templateLanguage = campaign.Template.Language
	}

	rows := [][]interface{}{
		{"Campaign Name", campaign.Name},
		{"Status", string(campaign.Status)},
		{"Template", templateName},
		{"Template Language", templateLanguage},
		{"WhatsApp Account", campaign.WhatsAppAccount},
		{"Total Recipients", campaign.TotalRecipients},
		{"Sent", campaign.SentCount},
		{"Delivered", campaign.DeliveredCount},
		{"Read", campaign.ReadCount},
		{"Failed", campaign.FailedCount},
		{"Timezone", report.Timezone},
		{"Scheduled At", formatCampaignReportTime(campaign.ScheduledAt, location)},
		{"Started At", formatCampaignReportTime(campaign.StartedAt, location)},
		{"Completed At", formatCampaignReportTime(campaign.CompletedAt, location)},
		{"Header Media ID", campaign.HeaderMediaID},
		{"Header Media Filename", campaign.HeaderMediaFilename},
		{"Header Media MIME Type", campaign.HeaderMediaMimeType},
		{"Generated At", report.GeneratedAt.In(location).Format(campaignReportTimeLayout)},
	}

	if err := file.MergeCell(campaignReportSummarySheet, "A1", "B1"); err != nil {
		return err
	}
	if err := file.SetCellValue(campaignReportSummarySheet, "A1", "Campaign Delivery Report"); err != nil {
		return err
	}
	if err := file.SetCellStyle(campaignReportSummarySheet, "A1", "A1", titleStyle); err != nil {
		return err
	}
	if err := file.SetRowHeight(campaignReportSummarySheet, 1, 24); err != nil {
		return err
	}

	for i, row := range rows {
		rowNumber := i + 3
		labelCell := fmt.Sprintf("A%d", rowNumber)
		valueCell := fmt.Sprintf("B%d", rowNumber)
		if err := file.SetCellValue(campaignReportSummarySheet, labelCell, row[0]); err != nil {
			return err
		}
		if err := file.SetCellStyle(campaignReportSummarySheet, labelCell, labelCell, labelStyle); err != nil {
			return err
		}
		if err := file.SetCellValue(campaignReportSummarySheet, valueCell, row[1]); err != nil {
			return err
		}
	}

	if err := file.SetColWidth(campaignReportSummarySheet, "A", "A", 24); err != nil {
		return err
	}
	if err := file.SetColWidth(campaignReportSummarySheet, "B", "B", 38); err != nil {
		return err
	}
	return nil
}

func populateCampaignRecipientsSheet(file *excelize.File, report campaignReportData, headerStyle int, statusStyles map[models.MessageStatus]int) error {
	location := campaignReportLocation(report.Location)
	headers := []string{
		"Phone Number",
		"Recipient Name",
		"Status",
		"Sent At",
		"Delivered At",
		"Read At",
		"WhatsApp Message ID",
		"Message ID",
		"Error Message",
		"Template Params",
		"Added At",
	}
	for i, header := range headers {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			return err
		}
		if err := file.SetCellValue(campaignReportRecipientsSheet, cell, header); err != nil {
			return err
		}
	}
	if err := file.SetCellStyle(campaignReportRecipientsSheet, "A1", "K1", headerStyle); err != nil {
		return err
	}
	if err := file.SetRowHeight(campaignReportRecipientsSheet, 1, 22); err != nil {
		return err
	}
	if err := file.AutoFilter(campaignReportRecipientsSheet, "A1:K1", nil); err != nil {
		return err
	}
	if err := file.SetPanes(campaignReportRecipientsSheet, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	}); err != nil {
		return err
	}

	columnWidths := map[string]float64{
		"A": 18,
		"B": 24,
		"C": 14,
		"D": 20,
		"E": 20,
		"F": 20,
		"G": 28,
		"H": 38,
		"I": 36,
		"J": 40,
		"K": 20,
	}
	for column, width := range columnWidths {
		if err := file.SetColWidth(campaignReportRecipientsSheet, column, column, width); err != nil {
			return err
		}
	}

	for i, recipient := range report.Recipients {
		row := i + 2
		values := []interface{}{
			recipient.PhoneNumber,
			recipient.RecipientName,
			string(recipient.Status),
			formatCampaignReportTime(recipient.SentAt, location),
			formatCampaignReportTime(recipient.DeliveredAt, location),
			formatCampaignReportTime(recipient.ReadAt, location),
			recipient.WhatsAppMessageID,
			formatCampaignReportMessageID(recipient.MessageID),
			recipient.ErrorMessage,
			formatCampaignReportTemplateParams(recipient.TemplateParams),
			recipient.CreatedAt.In(location).Format(campaignReportTimeLayout),
		}
		for col, value := range values {
			cell, err := excelize.CoordinatesToCellName(col+1, row)
			if err != nil {
				return err
			}
			if err := file.SetCellValue(campaignReportRecipientsSheet, cell, value); err != nil {
				return err
			}
		}
		if styleID, ok := statusStyles[recipient.Status]; ok {
			statusCell := fmt.Sprintf("C%d", row)
			if err := file.SetCellStyle(campaignReportRecipientsSheet, statusCell, statusCell, styleID); err != nil {
				return err
			}
		}
	}

	return nil
}

func formatCampaignReportTime(value *time.Time, location *time.Location) string {
	if value == nil {
		return ""
	}
	return value.In(campaignReportLocation(location)).Format(campaignReportTimeLayout)
}

func campaignReportLocation(location *time.Location) *time.Location {
	if location == nil {
		return time.UTC
	}
	return location
}

func formatCampaignReportMessageID(messageID *uuid.UUID) string {
	if messageID == nil {
		return ""
	}
	return messageID.String()
}

func formatCampaignReportTemplateParams(params models.JSONB) string {
	if len(params) == 0 {
		return ""
	}
	data, err := json.Marshal(params)
	if err != nil {
		return ""
	}
	return string(data)
}
