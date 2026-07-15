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
	Skipped  int
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
	result.Skipped = len(rows) - len(freshRows)
	if len(freshRows) == 0 {
		return result, nil
	}

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
		if row.Contact == nil {
			result.Skipped++
			continue
		}
		recipientID := uuid.New()
		resolved := resolveRSVPReminderParams(templateParams, event, row)
		recipientName := strings.TrimSpace(row.Contact.ProfileName)
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
