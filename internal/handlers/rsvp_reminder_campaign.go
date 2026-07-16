package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"gorm.io/gorm"
)

type rsvpReminderCampaignResult struct {
	Campaign *models.BulkMessageCampaign
	Queued   int
	Skipped  []rsvpReminderSkip
}

// rsvpReminderSkip records a guest that was not queued for a reminder, and why.
// Previously nil-contact rows were dropped with result.Skipped++ and no other
// trace (rsvp_reminder_campaign.go:81 before this change) — no delivery row, no
// log, no reason the admin could see.
type rsvpReminderSkip struct {
	ResponseID uuid.UUID `json:"response_id"`
	Name       string    `json:"name"`
	Phone      string    `json:"phone"`
	Reason     string    `json:"reason"`
}

// rsvpReminderSkipReason returns "" when a guest can be sent to, otherwise a
// human-readable reason. Preview and send both call this so their counts cannot
// drift: previously send dropped nil-contact rows (rsvp_reminder_campaign.go:81)
// while preview counted them as eligible (rsvp_reminders.go:168).
func rsvpReminderSkipReason(hasContact bool, phone string) string {
	if !hasContact {
		return "no contact record"
	}
	if normalizeRSVPReminderPhone(phone) == "" {
		return "no usable phone number"
	}
	return ""
}

// rsvpReminderRowName returns the guest's contact name, or "" if there is no
// contact to name (RSVPResponse has no RecipientName() helper).
func rsvpReminderRowName(row *models.RSVPResponse) string {
	if row.Contact == nil {
		return ""
	}
	return strings.TrimSpace(row.Contact.ProfileName)
}

// dedupeRSVPReminderRowsWithSkips wraps dedupeRSVPReminderRows and turns the
// dropped duplicates into skip records, so preview (RSVPReminderPreview) and
// send (createRSVPReminderCampaign) record duplicate phones identically.
func dedupeRSVPReminderRowsWithSkips(rows []models.RSVPResponse) (kept []models.RSVPResponse, skipped []rsvpReminderSkip) {
	var duplicates []models.RSVPResponse
	kept, duplicates = dedupeRSVPReminderRows(rows, func(r models.RSVPResponse) string { return r.PhoneNumber })
	skipped = make([]rsvpReminderSkip, 0, len(duplicates))
	for _, dup := range duplicates {
		skipped = append(skipped, rsvpReminderSkip{
			ResponseID: dup.ID,
			Name:       rsvpReminderRowName(&dup),
			Phone:      dup.PhoneNumber,
			Reason:     "duplicate phone number",
		})
	}
	return kept, skipped
}

// rsvpReminderEligibility applies the same dedupe and skip predicate the send
// path uses (dedupeRSVPReminderRowsWithSkips, rsvpReminderSkipReason) to a set
// of loaded rows, without the staleness recheck send does immediately before
// queuing (createRSVPReminderCampaign's freshRows reload). Preview has no
// equivalent of that recheck, so this matches predicate and reporting only —
// the two paths must report duplicates identically.
func rsvpReminderEligibility(rows []models.RSVPResponse) (eligible int, skipped []rsvpReminderSkip) {
	kept, dupSkips := dedupeRSVPReminderRowsWithSkips(rows)
	skipped = append(skipped, dupSkips...)
	for _, row := range kept {
		if reason := rsvpReminderSkipReason(row.Contact != nil, row.PhoneNumber); reason != "" {
			skipped = append(skipped, rsvpReminderSkip{
				ResponseID: row.ID,
				Name:       rsvpReminderRowName(&row),
				Phone:      row.PhoneNumber,
				Reason:     reason,
			})
			continue
		}
		eligible++
	}
	return eligible, skipped
}

// rsvpReminderCampaignOutcome classifies a finished reminder campaign. A run where
// every recipient failed must not present as a clean success — that is how 1008
// consecutive failures went unnoticed on 15/07/2026.
func rsvpReminderCampaignOutcome(sent, failed, total int) string {
	switch {
	case total > 0 && sent == 0 && failed >= total:
		return "failed"
	case failed > 0:
		return "completed_with_errors"
	default:
		return "completed"
	}
}

func rsvpReminderCampaignName(eventName string, now time.Time) string {
	name := fmt.Sprintf("RSVP Reminder - %s - %s", strings.TrimSpace(eventName), now.UTC().Format("2006-01-02 15:04 UTC"))
	runes := []rune(name)
	if len(runes) > 255 {
		name = string(runes[:255])
	}
	return name
}

// createRSVPReminderCampaign snapshots the currently eligible guests into a
// linked campaign. The worker rechecks RSVP eligibility immediately before
// each send and synchronizes its result to RSVPReminderDelivery.
func (a *App) createRSVPReminderCampaign(
	ctx context.Context,
	event *models.RSVPEvent,
	template *models.Template,
	templateParams map[string]string,
	rows []models.RSVPResponse,
	deliveryType models.RSVPReminderDeliveryType,
	scheduleID *uuid.UUID,
	createdBy uuid.UUID,
) (rsvpReminderCampaignResult, error) {
	result := rsvpReminderCampaignResult{}
	if len(rows) == 0 {
		return result, nil
	}
	if a.Queue == nil {
		return result, fmt.Errorf("campaign queue is unavailable")
	}

	responseIDs := make([]uuid.UUID, 0, len(rows))
	for i := range rows {
		responseIDs = append(responseIDs, rows[i].ID)
	}
	freshRows, err := a.loadNotStartedRSVPGuests(event.OrganizationID, event.ID, responseIDs, nil)
	if err != nil {
		return result, err
	}
	// Rows present at preview time but missing from the recheck became ineligible
	// in the interim (e.g. the guest responded or started RSVP) — report why
	// rather than folding them into a bare count.
	freshIDs := make(map[uuid.UUID]struct{}, len(freshRows))
	for i := range freshRows {
		freshIDs[freshRows[i].ID] = struct{}{}
	}
	for i := range rows {
		if _, ok := freshIDs[rows[i].ID]; ok {
			continue
		}
		result.Skipped = append(result.Skipped, rsvpReminderSkip{
			ResponseID: rows[i].ID,
			Name:       rsvpReminderRowName(&rows[i]),
			Phone:      rows[i].PhoneNumber,
			Reason:     "no longer eligible for a reminder",
		})
	}
	if len(freshRows) == 0 {
		return result, nil
	}

	var dupSkips []rsvpReminderSkip
	freshRows, dupSkips = dedupeRSVPReminderRowsWithSkips(freshRows)
	result.Skipped = append(result.Skipped, dupSkips...)

	now := time.Now().UTC()
	campaign := models.BulkMessageCampaign{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  event.OrganizationID,
		WhatsAppAccount: event.WhatsAppAccount,
		Name:            rsvpReminderCampaignName(event.Name, now),
		TemplateID:      template.ID,
		Status:          models.CampaignStatusDraft,
		TotalRecipients: len(freshRows),
		CreatedBy:       createdBy,
		SourceType:      models.CampaignSourceRSVPReminder,
		SourceID:        &event.ID,
	}

	recipients := make([]models.BulkMessageRecipient, 0, len(freshRows))
	deliveries := make([]models.RSVPReminderDelivery, 0, len(freshRows))
	for i := range freshRows {
		row := &freshRows[i]
		if reason := rsvpReminderSkipReason(row.Contact != nil, row.PhoneNumber); reason != "" {
			result.Skipped = append(result.Skipped, rsvpReminderSkip{
				ResponseID: row.ID,
				Name:       rsvpReminderRowName(row),
				Phone:      row.PhoneNumber,
				Reason:     reason,
			})
			a.Log.Warn("RSVP reminder skipped guest",
				"rsvp_response_id", row.ID, "reason", reason, "event_id", event.ID)
			continue
		}
		recipientID := uuid.New()
		resolved := resolveRSVPReminderParams(templateParams, event, row)
		recipientName := rsvpReminderRowName(row)
		if recipientName == "" {
			recipientName = row.PhoneNumber
		}
		recipients = append(recipients, models.BulkMessageRecipient{
			BaseModel:      models.BaseModel{ID: recipientID},
			CampaignID:     campaign.ID,
			PhoneNumber:    row.PhoneNumber,
			RecipientName:  recipientName,
			TemplateParams: rsvpReminderParamsJSON(resolved),
			Status:         models.MessageStatusPending,
		})
		campaignID := campaign.ID
		deliveries = append(deliveries, models.RSVPReminderDelivery{
			BaseModel:           models.BaseModel{ID: uuid.New()},
			RSVPEventID:         event.ID,
			OrganizationID:      event.OrganizationID,
			RSVPResponseID:      row.ID,
			ScheduleID:          scheduleID,
			DeliveryType:        deliveryType,
			Status:              models.RSVPReminderDeliveryQueued,
			AttemptedAt:         now,
			InitiatedBy:         reminderInitiator(deliveryType, createdBy),
			CampaignID:          &campaignID,
			CampaignRecipientID: &recipientID,
		})
	}
	if len(recipients) == 0 {
		return result, nil
	}
	campaign.TotalRecipients = len(recipients)

	err = a.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&campaign).Error; err != nil {
			return err
		}
		if err := tx.CreateInBatches(&recipients, campaignRecipientCreateBatchSize).Error; err != nil {
			return err
		}
		if err := tx.CreateInBatches(&deliveries, campaignRecipientCreateBatchSize).Error; err != nil {
			return err
		}
		if scheduleID != nil {
			if err := tx.Model(&models.RSVPReminderSchedule{}).
				Where("id = ? AND organization_id = ?", *scheduleID, event.OrganizationID).
				Update("campaign_id", campaign.ID).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return result, err
	}

	result.Campaign = &campaign
	result.Queued = len(recipients)

	// The RSVP path calls enqueueCampaignRecipients directly and so never passed
	// through StartCampaign's gate (campaigns.go:577). Without this, a media-header
	// template fails once per recipient with Meta error 132012 — 1008 times on
	// 15/07/2026, while the campaign reported "completed".
	if err := a.validateCampaignReadyForStart(&campaign); err != nil {
		return result, err
	}
	if err := a.enqueueCampaignRecipients(ctx, &campaign, recipients, now, models.CampaignStatusDraft); err != nil {
		return result, err
	}
	return result, nil
}

func reminderInitiator(deliveryType models.RSVPReminderDeliveryType, userID uuid.UUID) *uuid.UUID {
	if deliveryType != models.RSVPReminderDeliveryManual {
		return nil
	}
	return &userID
}
