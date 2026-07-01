package handlers

import (
	"context"
	"time"

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
	events := a.dueRSVPReminderEvents()
	for i := range events {
		event := &events[i]
		var pending []models.RSVPResponse
		a.DB.Where("rsvp_event_id = ? AND attendance = ?", event.ID, models.RSVPAttendancePending).
			Find(&pending)

		for j := range pending {
			p := &pending[j]
			if event.ReminderTemplateID == nil || event.WhatsAppAccount == "" {
				continue
			}
			var contact models.Contact
			if err := a.DB.Where("id = ?", p.ContactID).First(&contact).Error; err != nil {
				continue
			}
			a.sendRSVPInviteTemplate(event, event.ReminderTemplateID, &contact)
		}
		now := time.Now()
		a.DB.Model(event).Update("reminder_sent_at", now)
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
