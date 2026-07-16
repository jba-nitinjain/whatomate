package handlers

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
)

func (a *App) dueRSVPReminderEvents() []models.RSVPEvent {
	now := time.Now()
	var events []models.RSVPEvent
	a.DB.Where(`status = ? AND reminder_enabled = ? AND reminder_at IS NOT NULL
		AND reminder_at <= ? AND reminder_sent_at IS NULL
		AND (rsvp_close_at IS NULL OR rsvp_close_at > ?)`,
		models.RSVPEventStatusActive, true, now, now).
		Find(&events)
	return events
}

// ProcessDueRSVPReminders re-sends invites to pending guests for due events.
func (a *App) ProcessDueRSVPReminders(ctx context.Context) {
	a.backfillLegacyRSVPReminderSchedules()
	now := time.Now().UTC()
	var schedules []models.RSVPReminderSchedule
	a.DB.Where("status = ? AND scheduled_at <= ?", models.RSVPReminderSchedulePending, now).
		Order("scheduled_at").Find(&schedules)
	for i := range schedules {
		select {
		case <-ctx.Done():
			return
		default:
		}
		schedule := &schedules[i]
		claim := a.DB.Model(&models.RSVPReminderSchedule{}).
			Where("id = ? AND status = ?", schedule.ID, models.RSVPReminderSchedulePending).
			Update("status", models.RSVPReminderScheduleProcessing)
		if claim.Error != nil || claim.RowsAffected == 0 {
			continue
		}
		var event models.RSVPEvent
		if err := a.DB.Where("id = ? AND organization_id = ? AND status = ? AND (rsvp_close_at IS NULL OR rsvp_close_at > ?)", schedule.RSVPEventID, schedule.OrganizationID, models.RSVPEventStatusActive, now).First(&event).Error; err != nil {
			a.DB.Model(schedule).Updates(map[string]interface{}{"status": models.RSVPReminderScheduleCompletedWithErrors, "failed_count": 1, "processed_at": now})
			continue
		}
		rows, err := a.loadNotStartedRSVPGuests(schedule.OrganizationID, schedule.RSVPEventID, nil, nil)
		if err != nil {
			a.DB.Model(schedule).Updates(map[string]interface{}{"status": models.RSVPReminderScheduleCompletedWithErrors, "failed_count": 1, "processed_at": now})
			continue
		}
		if len(rows) == 0 {
			processed := time.Now().UTC()
			a.DB.Model(schedule).Updates(map[string]interface{}{"status": models.RSVPReminderScheduleCompleted, "processed_at": processed})
			continue
		}
		templateRaw := schedule.TemplateID.String()
		_, template, err := a.rsvpReminderTemplate(schedule.OrganizationID, &event, &templateRaw)
		if err != nil {
			a.DB.Model(schedule).Updates(map[string]interface{}{"status": models.RSVPReminderScheduleCompletedWithErrors, "failed_count": len(rows), "processed_at": now})
			continue
		}
		campaignResult, err := a.createRSVPReminderCampaign(ctx, &event, template, jsonbToStringMap(schedule.TemplateParams), rows, models.RSVPReminderDeliveryScheduled, &schedule.ID, schedule.CreatedBy, "", "", "")
		if err != nil {
			a.Log.Error("Failed to create scheduled RSVP reminder campaign", "schedule_id", schedule.ID, "error", err)
			a.DB.Model(schedule).Updates(map[string]interface{}{"status": models.RSVPReminderScheduleCompletedWithErrors, "failed_count": len(rows), "processed_at": now})
			continue
		}
		if campaignResult.Campaign == nil {
			processed := time.Now().UTC()
			a.DB.Model(schedule).Updates(map[string]interface{}{"status": models.RSVPReminderScheduleCompleted, "processed_at": processed})
		}
	}
}

func (a *App) backfillLegacyRSVPReminderSchedules() {
	var events []models.RSVPEvent
	a.DB.Where("reminder_enabled = ? AND reminder_at IS NOT NULL AND reminder_sent_at IS NULL AND reminder_template_id IS NOT NULL", true).Find(&events)
	for i := range events {
		event := &events[i]
		var count int64
		a.DB.Model(&models.RSVPReminderSchedule{}).Where("rsvp_event_id = ? AND scheduled_at = ?", event.ID, *event.ReminderAt).Count(&count)
		if count == 0 {
			a.DB.Create(&models.RSVPReminderSchedule{BaseModel: models.BaseModel{ID: uuid.New()}, RSVPEventID: event.ID, OrganizationID: event.OrganizationID, ScheduledAt: event.ReminderAt.UTC(), TemplateID: *event.ReminderTemplateID, Status: models.RSVPReminderSchedulePending, CreatedBy: event.CreatedBy})
		}
		a.DB.Model(event).Update("reminder_enabled", false)
	}
}

// StartRSVPReminderProcessor runs ProcessDueRSVPReminders on a ticker, mirroring
// StartScheduledCampaignProcessor.
func (a *App) StartRSVPReminderProcessor(interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.RSVPReminderCancel = cancel

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		a.ProcessDueRSVPReminders(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.ProcessDueRSVPReminders(ctx)
			}
		}
	}()
}

// DueRSVPReminderEventsForTest exposes dueRSVPReminderEvents for tests.
func (a *App) DueRSVPReminderEventsForTest() []models.RSVPEvent { return a.dueRSVPReminderEvents() }
