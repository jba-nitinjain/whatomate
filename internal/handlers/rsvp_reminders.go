package handlers

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

type rsvpReminderSendRequest struct {
	ResponseIDs   []string `json:"response_ids"`
	AllNotStarted bool     `json:"all_not_started"`
	TemplateID    *string  `json:"template_id"`
}

type rsvpReminderScheduleRequest struct {
	ScheduledAt time.Time `json:"scheduled_at"`
	TemplateID  string    `json:"template_id"`
}

func (a *App) rsvpReminderTemplate(orgID uuid.UUID, event *models.RSVPEvent, raw *string) (*uuid.UUID, error) {
	templateID := event.ReminderTemplateID
	if raw != nil && strings.TrimSpace(*raw) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(*raw))
		if err != nil {
			return nil, err
		}
		templateID = &parsed
	}
	if templateID == nil {
		return nil, &rsvpError{"reminder template is required"}
	}
	var template models.Template
	if err := a.DB.Where("id = ? AND organization_id = ? AND whats_app_account = ?", *templateID, orgID, event.WhatsAppAccount).First(&template).Error; err != nil {
		return nil, &rsvpError{"reminder template was not found for this WhatsApp account"}
	}
	if !strings.EqualFold(template.Status, "APPROVED") {
		return nil, &rsvpError{"reminder template must be approved"}
	}
	return templateID, nil
}

func (a *App) loadNotStartedRSVPGuests(orgID, eventID uuid.UUID, responseIDs []uuid.UUID) ([]models.RSVPResponse, error) {
	q := a.DB.Where("organization_id = ? AND rsvp_event_id = ? AND rsvp_started_at IS NULL AND responded_at IS NULL", orgID, eventID)
	if len(responseIDs) > 0 {
		q = q.Where("id IN ?", responseIDs)
	}
	var rows []models.RSVPResponse
	err := q.Preload("Contact").Find(&rows).Error
	return rows, err
}

func parseRSVPResponseIDs(values []string) ([]uuid.UUID, int) {
	ids := make([]uuid.UUID, 0, len(values))
	invalid := 0
	seen := map[uuid.UUID]bool{}
	for _, raw := range values {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil || seen[id] {
			invalid++
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, invalid
}

func (a *App) RSVPReminderPreview(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	if err := a.requirePermission(r, userID, models.ResourceRSVP, models.ActionRead); err != nil {
		return nil
	}
	eventID, err := parsePathUUID(r, "id", "RSVP event")
	if err != nil {
		return nil
	}
	event, err := findByIDAndOrg[models.RSVPEvent](a.DB, r, eventID, orgID, "RSVP event")
	if err != nil {
		return nil
	}
	raw := strings.TrimSpace(string(r.RequestCtx.QueryArgs().Peek("response_ids")))
	parts := []string{}
	if raw != "" {
		parts = strings.Split(raw, ",")
	}
	ids, invalid := parseRSVPResponseIDs(parts)
	rows, loadErr := a.loadNotStartedRSVPGuests(orgID, eventID, ids)
	if loadErr != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to preview reminders", nil, "")
	}
	configured := event.WhatsAppAccount != "" && event.ReminderTemplateID != nil
	return r.SendEnvelope(map[string]interface{}{"eligible": len(rows), "ineligible": maxInt(0, len(parts)-len(rows)), "invalid": invalid, "configured": configured})
}

func (a *App) sendRSVPReminders(event *models.RSVPEvent, templateID *uuid.UUID, rows []models.RSVPResponse, deliveryType models.RSVPReminderDeliveryType, scheduleID *uuid.UUID, initiatedBy *uuid.UUID) map[string]interface{} {
	sent, failed, skipped := 0, 0, 0
	errors := []map[string]string{}
	for i := range rows {
		row := &rows[i]
		// Reserve a scheduled delivery before sending. The unique index makes the
		// scheduler idempotent across concurrent processors.
		delivery := models.RSVPReminderDelivery{BaseModel: models.BaseModel{ID: uuid.New()}, RSVPEventID: event.ID, OrganizationID: event.OrganizationID, RSVPResponseID: row.ID, ScheduleID: scheduleID, DeliveryType: deliveryType, Status: models.RSVPReminderDeliverySkipped, AttemptedAt: time.Now().UTC(), InitiatedBy: initiatedBy}
		if err := a.DB.Create(&delivery).Error; err != nil {
			skipped++
			continue
		}
		// Eligibility may have changed since preview/schedule selection.
		var fresh models.RSVPResponse
		if err := a.DB.Preload("Contact").Where("id = ? AND organization_id = ? AND rsvp_started_at IS NULL AND responded_at IS NULL", row.ID, event.OrganizationID).First(&fresh).Error; err != nil || fresh.Contact == nil {
			skipped++
			continue
		}
		messageID, err := a.sendRSVPInviteTemplate(event, templateID, fresh.Contact)
		if err != nil {
			failed++
			a.DB.Model(&delivery).Updates(map[string]interface{}{"status": models.RSVPReminderDeliveryFailed, "error_message": err.Error()})
			errors = append(errors, map[string]string{"response_id": row.ID.String(), "error": err.Error()})
			continue
		}
		sent++
		a.DB.Model(&delivery).Updates(map[string]interface{}{"status": models.RSVPReminderDeliverySent, "message_id": messageID})
	}
	return map[string]interface{}{"requested": len(rows), "sent": sent, "failed": failed, "skipped": skipped, "errors": errors}
}

func (a *App) SendRSVPReminders(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	if err := a.requirePermission(r, userID, models.ResourceRSVP, models.ActionExecute); err != nil {
		return nil
	}
	eventID, err := parsePathUUID(r, "id", "RSVP event")
	if err != nil {
		return nil
	}
	event, err := findByIDAndOrg[models.RSVPEvent](a.DB, r, eventID, orgID, "RSVP event")
	if err != nil {
		return nil
	}
	var req rsvpReminderSendRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	ids, invalid := parseRSVPResponseIDs(req.ResponseIDs)
	if !req.AllNotStarted && len(ids) == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Select guests or request all not-started guests", nil, "")
	}
	templateID, err := a.rsvpReminderTemplate(orgID, event, req.TemplateID)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}
	if req.AllNotStarted {
		ids = nil
	}
	rows, err := a.loadNotStartedRSVPGuests(orgID, eventID, ids)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to load reminder recipients", nil, "")
	}
	result := a.sendRSVPReminders(event, templateID, rows, models.RSVPReminderDeliveryManual, nil, &userID)
	result["skipped"] = result["skipped"].(int) + invalid + maxInt(0, len(ids)-len(rows))
	if req.AllNotStarted {
		result["requested"] = len(rows)
	} else {
		result["requested"] = len(req.ResponseIDs)
	}
	return r.SendEnvelope(result)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (a *App) ListRSVPReminderSchedules(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	if err := a.requirePermission(r, userID, models.ResourceRSVP, models.ActionRead); err != nil {
		return nil
	}
	eventID, err := parsePathUUID(r, "id", "RSVP event")
	if err != nil {
		return nil
	}
	if _, err = findByIDAndOrg[models.RSVPEvent](a.DB, r, eventID, orgID, "RSVP event"); err != nil {
		return nil
	}
	var rows []models.RSVPReminderSchedule
	if err := a.DB.Where("organization_id = ? AND rsvp_event_id = ?", orgID, eventID).Order("scheduled_at").Find(&rows).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to list reminder schedules", nil, "")
	}
	return r.SendEnvelope(map[string]interface{}{"reminders": rows})
}

func (a *App) CreateRSVPReminderSchedule(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	if err := a.requirePermission(r, userID, models.ResourceRSVP, models.ActionWrite); err != nil {
		return nil
	}
	eventID, err := parsePathUUID(r, "id", "RSVP event")
	if err != nil {
		return nil
	}
	event, err := findByIDAndOrg[models.RSVPEvent](a.DB, r, eventID, orgID, "RSVP event")
	if err != nil {
		return nil
	}
	var req rsvpReminderScheduleRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	if req.ScheduledAt.IsZero() || !req.ScheduledAt.After(time.Now()) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "scheduled_at must be in the future", nil, "")
	}
	templateRaw := req.TemplateID
	templateID, err := a.rsvpReminderTemplate(orgID, event, &templateRaw)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}
	schedule := models.RSVPReminderSchedule{BaseModel: models.BaseModel{ID: uuid.New()}, RSVPEventID: eventID, OrganizationID: orgID, ScheduledAt: req.ScheduledAt.UTC(), TemplateID: *templateID, Status: models.RSVPReminderSchedulePending, CreatedBy: userID}
	if err := a.DB.Create(&schedule).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to schedule reminder", nil, "")
	}
	return r.SendEnvelope(schedule)
}

func (a *App) CancelRSVPReminderSchedule(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	if err := a.requirePermission(r, userID, models.ResourceRSVP, models.ActionWrite); err != nil {
		return nil
	}
	eventID, err := parsePathUUID(r, "id", "RSVP event")
	if err != nil {
		return nil
	}
	scheduleID, err := parsePathUUID(r, "scheduleId", "reminder schedule")
	if err != nil {
		return nil
	}
	res := a.DB.Model(&models.RSVPReminderSchedule{}).Where("id = ? AND rsvp_event_id = ? AND organization_id = ? AND status = ?", scheduleID, eventID, orgID, models.RSVPReminderSchedulePending).Update("status", models.RSVPReminderScheduleCancelled)
	if res.Error != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to cancel reminder", nil, "")
	}
	if res.RowsAffected == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusConflict, "Only pending reminders can be cancelled", nil, "")
	}
	return r.SendEnvelope(map[string]bool{"cancelled": true})
}
