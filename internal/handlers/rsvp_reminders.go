package handlers

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/internal/templateutil"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

type rsvpReminderSendRequest struct {
	ResponseIDs        []string          `json:"response_ids"`
	ExcludeResponseIDs []string          `json:"exclude_response_ids"`
	AllNotStarted      bool              `json:"all_not_started"`
	TemplateID         *string           `json:"template_id"`
	TemplateParams     map[string]string `json:"template_params"`
}

type rsvpReminderScheduleRequest struct {
	ScheduledAt    time.Time         `json:"scheduled_at"`
	TemplateID     string            `json:"template_id"`
	TemplateParams map[string]string `json:"template_params"`
}

func rsvpReminderRequiredParams(template *models.Template) []string {
	seen := map[string]bool{}
	params := []string{}
	for _, name := range append(templateutil.ExtParamNames(template.BodyContent), templateutil.ExtractURLButtonParamNames(template.Buttons)...) {
		if name != "" && !seen[name] {
			seen[name] = true
			params = append(params, name)
		}
	}
	return params
}

func validateRSVPReminderParams(template *models.Template, params map[string]string) error {
	missing := []string{}
	for _, name := range rsvpReminderRequiredParams(template) {
		if strings.TrimSpace(params[name]) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("map reminder template parameters: %s", strings.Join(missing, ", "))
	}
	return nil
}

func rsvpReminderParamsJSON(params map[string]string) models.JSONB {
	out := models.JSONB{}
	for key, value := range params {
		out[key] = value
	}
	return out
}

func resolveRSVPReminderParams(params map[string]string, event *models.RSVPEvent, response *models.RSVPResponse) map[string]string {
	if len(params) == 0 {
		return nil
	}
	memberName := response.PhoneNumber
	if response.Contact != nil && strings.TrimSpace(response.Contact.ProfileName) != "" {
		memberName = response.Contact.ProfileName
	}
	eventDate := ""
	if event.EventDate != nil {
		eventDate = event.EventDate.Format("02/01/2006")
	}
	replacer := strings.NewReplacer(
		"{{member_name}}", memberName, "{{member.name}}", memberName,
		"{{member_phone}}", response.PhoneNumber, "{{member.phone}}", response.PhoneNumber,
		"{{event_name}}", event.Name, "{{event.name}}", event.Name,
		"{{event_date}}", eventDate, "{{event.date}}", eventDate,
		"{{event_description}}", event.Description, "{{event.description}}", event.Description,
		"{{event_keyword}}", event.Keyword, "{{event.keyword}}", event.Keyword,
	)
	resolved := make(map[string]string, len(params))
	for key, raw := range params {
		value := replacer.Replace(raw)
		for answerKey, answerValue := range response.Answers {
			value = strings.ReplaceAll(value, "{{answer."+answerKey+"}}", fmt.Sprint(answerValue))
		}
		resolved[key] = value
	}
	return resolved
}

func (a *App) rsvpReminderTemplate(orgID uuid.UUID, event *models.RSVPEvent, raw *string) (*uuid.UUID, *models.Template, error) {
	templateID := event.ReminderTemplateID
	if raw != nil && strings.TrimSpace(*raw) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(*raw))
		if err != nil {
			return nil, nil, err
		}
		templateID = &parsed
	}
	if templateID == nil {
		return nil, nil, &rsvpError{"reminder template is required"}
	}
	var template models.Template
	if err := a.DB.Where("id = ? AND organization_id = ? AND whats_app_account = ?", *templateID, orgID, event.WhatsAppAccount).First(&template).Error; err != nil {
		return nil, nil, &rsvpError{"reminder template was not found for this WhatsApp account"}
	}
	if !strings.EqualFold(template.Status, "APPROVED") {
		return nil, nil, &rsvpError{"reminder template must be approved"}
	}
	return templateID, &template, nil
}

func (a *App) loadNotStartedRSVPGuests(orgID, eventID uuid.UUID, responseIDs, excludeResponseIDs []uuid.UUID) ([]models.RSVPResponse, error) {
	q := a.DB.Where("organization_id = ? AND rsvp_event_id = ? AND rsvp_started_at IS NULL AND responded_at IS NULL", orgID, eventID)
	if len(responseIDs) > 0 {
		q = q.Where("id IN ?", responseIDs)
	}
	if len(excludeResponseIDs) > 0 {
		q = q.Where("id NOT IN ?", excludeResponseIDs)
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
	rows, loadErr := a.loadNotStartedRSVPGuests(orgID, eventID, ids, nil)
	if loadErr != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to preview reminders", nil, "")
	}
	// Apply the same dedupe and predicate the send path uses
	// (dedupeRSVPReminderRowsWithSkips, rsvpReminderSkipReason) so this preview
	// cannot promise more recipients than send will actually queue.
	eligible, skipped := rsvpReminderEligibility(rows)
	configured := event.WhatsAppAccount != "" && event.ReminderTemplateID != nil
	return r.SendEnvelope(map[string]interface{}{
		"eligible":   eligible,
		"ineligible": maxInt(0, len(parts)-len(rows)) + len(skipped),
		"skipped":    skipped,
		"invalid":    invalid,
		"configured": configured,
	})
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
	excludedIDs, _ := parseRSVPResponseIDs(req.ExcludeResponseIDs)
	if !req.AllNotStarted && len(ids) == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Select guests or request all not-started guests", nil, "")
	}
	_, template, err := a.rsvpReminderTemplate(orgID, event, req.TemplateID)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}
	if err := validateRSVPReminderParams(template, req.TemplateParams); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}
	if req.AllNotStarted {
		ids = nil
	}
	rows, err := a.loadNotStartedRSVPGuests(orgID, eventID, ids, excludedIDs)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to load reminder recipients", nil, "")
	}
	campaignResult, err := a.createRSVPReminderCampaign(r.RequestCtx, event, template, req.TemplateParams, rows, models.RSVPReminderDeliveryManual, nil, userID)
	if err != nil {
		a.Log.Error("Failed to create RSVP reminder campaign", "event_id", event.ID, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to create reminder campaign", nil, "")
	}
	requested := len(req.ResponseIDs)
	if req.AllNotStarted {
		requested = len(rows)
	}
	result := map[string]interface{}{
		"requested": requested,
		"queued":    campaignResult.Queued,
		"sent":      0,
		"failed":    0,
		"skipped":   len(campaignResult.Skipped) + invalid + maxInt(0, len(ids)-len(rows)),
	}
	if campaignResult.Campaign != nil {
		result["campaign_id"] = campaignResult.Campaign.ID
		result["campaign_name"] = campaignResult.Campaign.Name
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
	templateID, template, err := a.rsvpReminderTemplate(orgID, event, &templateRaw)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}
	if err := validateRSVPReminderParams(template, req.TemplateParams); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}
	schedule := models.RSVPReminderSchedule{BaseModel: models.BaseModel{ID: uuid.New()}, RSVPEventID: eventID, OrganizationID: orgID, ScheduledAt: req.ScheduledAt.UTC(), TemplateID: *templateID, TemplateParams: rsvpReminderParamsJSON(req.TemplateParams), Status: models.RSVPReminderSchedulePending, CreatedBy: userID}
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
