package handlers

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/xuri/excelize/v2"
	"github.com/zerodha/fastglue"

	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/internal/templateutil"
	"github.com/nikyjain/whatomate/pkg/whatsapp"
)

// ---------------------------------------------------------------------------
// Request types + helpers
// ---------------------------------------------------------------------------

type rsvpEventRequest struct {
	Name                  string                           `json:"name"`
	Description           string                           `json:"description"`
	EventDate             *time.Time                       `json:"event_date"`
	RSVPCloseAt           *time.Time                       `json:"rsvp_close_at"`
	WhatsAppAccount       string                           `json:"whatsapp_account"`
	FlowID                *string                          `json:"flow_id"`
	Keyword               string                           `json:"keyword"`
	AttendanceField       string                           `json:"attendance_field"`
	AttendanceMap         models.JSONB                     `json:"attendance_map"`
	SpouseMobileField     string                           `json:"spouse_mobile_field"`
	DuplicateMessage      string                           `json:"duplicate_message"`
	TemplateID            *string                          `json:"template_id"`
	ReminderEnabled       bool                             `json:"reminder_enabled"`
	ReminderAt            *time.Time                       `json:"reminder_at"`
	ReminderTemplateID    *string                          `json:"reminder_template_id"`
	AccessMode            *string                          `json:"access_mode"`
	NotInvitedMessage     *string                          `json:"not_invited_message"`
	HeadcountContributors models.RSVPHeadcountContributors `json:"headcount_contributors"`
}

const defaultRSVPNotInvitedMessage = "Sorry, this RSVP is limited to invited guests."

func validRSVPAccessMode(v models.RSVPAccessMode) bool {
	return v == models.RSVPAccessModeGuestList || v == models.RSVPAccessModeOpenKeyword
}

func parseOptionalUUID(s *string) (*uuid.UUID, bool) {
	if s == nil || *s == "" {
		return nil, true
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return nil, false
	}
	return &id, true
}

func (a *App) applyRSVPEventRequest(e *models.RSVPEvent, req rsvpEventRequest) error {
	e.Name = req.Name
	e.Description = req.Description
	e.EventDate = req.EventDate
	e.RSVPCloseAt = req.RSVPCloseAt
	e.WhatsAppAccount = req.WhatsAppAccount
	e.Keyword = req.Keyword
	if req.AttendanceField != "" {
		e.AttendanceField = req.AttendanceField
	} else if e.AttendanceField == "" {
		e.AttendanceField = "attendance"
	}
	if req.AttendanceMap != nil {
		e.AttendanceMap = req.AttendanceMap
	}
	e.SpouseMobileField = strings.TrimSpace(req.SpouseMobileField)
	e.DuplicateMessage = strings.TrimSpace(req.DuplicateMessage)
	e.ReminderEnabled = req.ReminderEnabled
	e.ReminderAt = req.ReminderAt
	if req.AccessMode != nil {
		mode := models.RSVPAccessMode(strings.TrimSpace(*req.AccessMode))
		if !validRSVPAccessMode(mode) {
			return fmt.Errorf("invalid access mode")
		}
		e.AccessMode = mode
	}
	if req.NotInvitedMessage != nil {
		e.NotInvitedMessage = strings.TrimSpace(*req.NotInvitedMessage)
	}
	if fid, ok := parseOptionalUUID(req.FlowID); ok {
		e.FlowID = fid
	} else {
		return fmt.Errorf("invalid flow ID")
	}
	if tid, ok := parseOptionalUUID(req.TemplateID); ok {
		e.TemplateID = tid
	} else {
		return fmt.Errorf("invalid template ID")
	}
	if rid, ok := parseOptionalUUID(req.ReminderTemplateID); ok {
		e.ReminderTemplateID = rid
	} else {
		return fmt.Errorf("invalid reminder template ID")
	}
	// Headcount contributors: only touched when the request explicitly sends the
	// field (nil means "leave whatever is already saved alone"), same as
	// AttendanceMap above. Validated against the event's own (possibly
	// just-updated) AttendanceField so the double-counting check below sees the
	// value that is about to be saved, not the stale one on disk.
	if req.HeadcountContributors != nil {
		if err := validateRSVPHeadcountContributors(req.HeadcountContributors, e.AttendanceField); err != nil {
			return err
		}
		e.HeadcountContributors = req.HeadcountContributors
	}
	return nil
}

// ---------------------------------------------------------------------------
// Event CRUD
// ---------------------------------------------------------------------------

func (a *App) ListRSVPEvents(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	pg := parsePagination(r)
	search := string(r.RequestCtx.QueryArgs().Peek("search"))
	status := string(r.RequestCtx.QueryArgs().Peek("status"))

	q := a.DB.Model(&models.RSVPEvent{}).Where("organization_id = ?", orgID)
	if search != "" {
		q = q.Where("name ILIKE ?", "%"+search+"%")
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	q.Count(&total)

	var events []models.RSVPEvent
	if err := pg.Apply(q.Order("created_at DESC")).Find(&events).Error; err != nil {
		a.Log.Error("Failed to list rsvp events", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to list RSVP events", nil, "")
	}
	return r.SendEnvelope(map[string]interface{}{
		"events": events,
		"total":  total,
		"page":   pg.Page,
		"limit":  pg.Limit,
	})
}

func (a *App) CreateRSVPEvent(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	var req rsvpEventRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	if req.Name == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Name is required", nil, "")
	}
	event := models.RSVPEvent{
		BaseModel:         models.BaseModel{ID: uuid.New()},
		OrganizationID:    orgID,
		Status:            models.RSVPEventStatusDraft,
		AttendanceField:   "attendance",
		CreatedBy:         userID,
		AccessMode:        models.RSVPAccessModeGuestList,
		NotInvitedMessage: defaultRSVPNotInvitedMessage,
	}
	if err := a.applyRSVPEventRequest(&event, req); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}
	if err := a.DB.Create(&event).Error; err != nil {
		a.Log.Error("Failed to create rsvp event", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to create RSVP event", nil, "")
	}
	return r.SendEnvelope(map[string]interface{}{"id": event.ID, "name": event.Name})
}

func (a *App) GetRSVPEvent(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	id, err := parsePathUUID(r, "id", "RSVP event")
	if err != nil {
		return nil
	}
	event, err := findByIDAndOrg[models.RSVPEvent](a.DB, r, id, orgID, "RSVP event")
	if err != nil {
		return nil
	}
	return r.SendEnvelope(event)
}

func (a *App) UpdateRSVPEvent(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	id, err := parsePathUUID(r, "id", "RSVP event")
	if err != nil {
		return nil
	}
	event, err := findByIDAndOrg[models.RSVPEvent](a.DB, r, id, orgID, "RSVP event")
	if err != nil {
		return nil
	}
	var req rsvpEventRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	if err := a.applyRSVPEventRequest(event, req); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}
	if event.Status == models.RSVPEventStatusActive {
		if verr := a.validateUniqueActiveKeyword(orgID, event.Keyword, event.ID); verr != nil {
			return r.SendErrorEnvelope(fasthttp.StatusConflict, verr.Error(), nil, "")
		}
	}
	if err := a.DB.Save(event).Error; err != nil {
		a.Log.Error("Failed to update rsvp event", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to update RSVP event", nil, "")
	}
	if event.Status == models.RSVPEventStatusActive {
		a.syncRSVPFlowKeyword(event)
	}
	return r.SendEnvelope(map[string]interface{}{"id": event.ID})
}

func (a *App) DeleteRSVPEvent(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	id, err := parsePathUUID(r, "id", "RSVP event")
	if err != nil {
		return nil
	}
	if err := a.DB.Where("id = ? AND organization_id = ?", id, orgID).
		Delete(&models.RSVPEvent{}).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to delete RSVP event", nil, "")
	}
	return r.SendEnvelope(map[string]interface{}{"deleted": true})
}

// ---------------------------------------------------------------------------
// Lifecycle: activate / close + unique-active-keyword rule
// ---------------------------------------------------------------------------

type rsvpError struct{ msg string }

func (e *rsvpError) Error() string { return e.msg }

var errKeywordInUse = &rsvpError{"keyword already used by another active event"}

func (a *App) validateUniqueActiveKeyword(orgID uuid.UUID, keyword string, excludeID uuid.UUID) error {
	if keyword == "" {
		return nil
	}
	var count int64
	a.DB.Model(&models.RSVPEvent{}).
		Where("organization_id = ? AND LOWER(keyword) = LOWER(?) AND status = ? AND id <> ?",
			orgID, keyword, models.RSVPEventStatusActive, excludeID).
		Count(&count)
	if count > 0 {
		return errKeywordInUse
	}
	return nil
}

// ValidateUniqueActiveKeywordForTest exposes validateUniqueActiveKeyword for tests.
func (a *App) ValidateUniqueActiveKeywordForTest(orgID uuid.UUID, keyword string, excludeID uuid.UUID) error {
	return a.validateUniqueActiveKeyword(orgID, keyword, excludeID)
}

// syncRSVPFlowKeyword ensures the linked flow's TriggerKeywords include the event keyword,
// so keyword/link entry starts the RSVP flow.
func (a *App) syncRSVPFlowKeyword(event *models.RSVPEvent) {
	if event.FlowID == nil || event.Keyword == "" {
		return
	}
	var flow models.ChatbotFlow
	if err := a.DB.Where("id = ? AND organization_id = ?", *event.FlowID, event.OrganizationID).
		First(&flow).Error; err != nil {
		return
	}
	for _, k := range flow.TriggerKeywords {
		if k == event.Keyword {
			return
		}
	}
	flow.TriggerKeywords = append(flow.TriggerKeywords, event.Keyword)
	a.DB.Model(&flow).Update("trigger_keywords", flow.TriggerKeywords)
	// Invalidate the chatbot flows cache so matchFlowTrigger sees the new keyword
	// immediately (the cache TTL is 6h). Mirrors UpdateChatbotFlow's behavior.
	a.InvalidateChatbotFlowsCache(event.OrganizationID)
}

func (a *App) ActivateRSVPEvent(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	id, err := parsePathUUID(r, "id", "RSVP event")
	if err != nil {
		return nil
	}
	event, err := findByIDAndOrg[models.RSVPEvent](a.DB, r, id, orgID, "RSVP event")
	if err != nil {
		return nil
	}
	if verr := a.validateUniqueActiveKeyword(orgID, event.Keyword, event.ID); verr != nil {
		return r.SendErrorEnvelope(fasthttp.StatusConflict, verr.Error(), nil, "")
	}
	event.Status = models.RSVPEventStatusActive
	if err := a.DB.Model(event).Update("status", event.Status).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to activate", nil, "")
	}
	a.syncRSVPFlowKeyword(event)
	return r.SendEnvelope(map[string]interface{}{"id": event.ID, "status": event.Status})
}

func (a *App) CloseRSVPEvent(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	id, err := parsePathUUID(r, "id", "RSVP event")
	if err != nil {
		return nil
	}
	event, err := findByIDAndOrg[models.RSVPEvent](a.DB, r, id, orgID, "RSVP event")
	if err != nil {
		return nil
	}
	event.Status = models.RSVPEventStatusClosed
	if err := a.DB.Model(event).Update("status", event.Status).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to close", nil, "")
	}
	return r.SendEnvelope(map[string]interface{}{"id": event.ID, "status": event.Status})
}

// ---------------------------------------------------------------------------
// Responses + tally
// ---------------------------------------------------------------------------

func (a *App) ListRSVPResponses(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	eventID, err := parsePathUUID(r, "id", "RSVP event")
	if err != nil {
		return nil
	}
	pg := parsePagination(r)
	q := a.DB.Model(&models.RSVPResponse{}).
		Where("rsvp_responses.organization_id = ? AND rsvp_responses.rsvp_event_id = ?", orgID, eventID)
	if status := string(r.RequestCtx.QueryArgs().Peek("attendance")); status != "" {
		q = q.Where("rsvp_responses.attendance = ?", status)
	}
	// Filter by dynamic "*_title" answer field values (used by clickable stat cards).
	// Two independent pairs may be combined (AND) — e.g. member Attending + spouse
	// Not Attending. title_value "__pending__" matches rows unanswered/empty.
	applyTitleFilter := func(field, value string) {
		if field == "" {
			return
		}
		if value == "__pending__" {
			q = q.Where("(rsvp_responses.answers->>? IS NULL OR rsvp_responses.answers->>? = '')", field, field)
		} else if value != "" {
			q = q.Where("rsvp_responses.answers->>? = ?", field, value)
		}
	}
	applyTitleFilter(string(r.RequestCtx.QueryArgs().Peek("title_field")), string(r.RequestCtx.QueryArgs().Peek("title_value")))
	applyTitleFilter(string(r.RequestCtx.QueryArgs().Peek("title_field2")), string(r.RequestCtx.QueryArgs().Peek("title_value2")))
	if search := strings.TrimSpace(string(r.RequestCtx.QueryArgs().Peek("search"))); search != "" {
		like := "%" + search + "%"
		q = q.Joins("LEFT JOIN contacts ON contacts.id = rsvp_responses.contact_id").
			Where("rsvp_responses.phone_number LIKE ? OR contacts.profile_name ILIKE ?", like, like)
	}
	var total int64
	q.Count(&total)
	var rows []models.RSVPResponse
	if err := pg.Apply(q.Preload("Contact").Order("responded_at DESC NULLS LAST")).Find(&rows).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to list responses", nil, "")
	}
	return r.SendEnvelope(map[string]interface{}{"responses": rows, "total": total, "page": pg.Page, "limit": pg.Limit})
}

type rsvpResponseUpdateRequest struct {
	Attendance *string                `json:"attendance"`
	Answers    map[string]interface{} `json:"answers"`
	Notes      *string                `json:"notes"`
}

func isValidRSVPAttendance(v string) bool {
	switch models.RSVPAttendance(v) {
	case models.RSVPAttendancePending, models.RSVPAttendanceYes, models.RSVPAttendanceNo, models.RSVPAttendanceMaybe:
		return true
	}
	return false
}

// UpdateRSVPResponse edits an existing received response (attendance, answers, notes).
func (a *App) UpdateRSVPResponse(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	eventID, err := parsePathUUID(r, "id", "RSVP event")
	if err != nil {
		return nil
	}
	respID, err := parsePathUUID(r, "responseId", "RSVP response")
	if err != nil {
		return nil
	}

	var resp models.RSVPResponse
	if err := a.DB.Where("id = ? AND organization_id = ? AND rsvp_event_id = ?", respID, orgID, eventID).
		First(&resp).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "RSVP response not found", nil, "")
	}

	var req rsvpResponseUpdateRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	if req.Attendance != nil {
		if !isValidRSVPAttendance(*req.Attendance) {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "attendance must be pending, yes, no, or maybe", nil, "")
		}
		resp.Attendance = models.RSVPAttendance(*req.Attendance)
		// Stamp responded_at the first time a real answer is recorded manually.
		if resp.Attendance != models.RSVPAttendancePending && resp.RespondedAt == nil {
			now := time.Now().UTC()
			resp.RespondedAt = &now
		}
	}
	if req.Answers != nil {
		resp.Answers = models.JSONB(req.Answers)
		if resp.RespondedAt == nil {
			now := time.Now().UTC()
			resp.RespondedAt = &now
		}
	}
	if req.Notes != nil {
		resp.Notes = strings.TrimSpace(*req.Notes)
	}

	if err := a.DB.Save(&resp).Error; err != nil {
		a.Log.Error("Failed to update rsvp response", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to update RSVP response", nil, "")
	}
	return r.SendEnvelope(resp)
}

// DeleteRSVPResponse removes a received response from an event.
func (a *App) DeleteRSVPResponse(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	eventID, err := parsePathUUID(r, "id", "RSVP event")
	if err != nil {
		return nil
	}
	respID, err := parsePathUUID(r, "responseId", "RSVP response")
	if err != nil {
		return nil
	}
	// Hard delete so the unique (event, contact) slot is freed and the guest can
	// RSVP again — a soft delete would leave the row blocking re-submission.
	if err := a.DB.Unscoped().Where("id = ? AND organization_id = ? AND rsvp_event_id = ?", respID, orgID, eventID).
		Delete(&models.RSVPResponse{}).Error; err != nil {
		a.Log.Error("Failed to delete rsvp response", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to delete RSVP response", nil, "")
	}
	return r.SendEnvelope(map[string]interface{}{"deleted": true})
}

func (a *App) GetRSVPTally(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	eventID, err := parsePathUUID(r, "id", "RSVP event")
	if err != nil {
		return nil
	}
	var ev models.RSVPEvent
	if err := a.DB.Select("attendance_field", "spouse_mobile_field", "headcount_contributors").
		Where("id = ? AND organization_id = ?", eventID, orgID).First(&ev).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "RSVP event not found", nil, "")
	}
	type row struct {
		Attendance models.RSVPAttendance
		Count      int
	}
	var rows []row
	a.DB.Model(&models.RSVPResponse{}).
		Select("attendance, count(*) as count").
		Where("organization_id = ? AND rsvp_event_id = ?", orgID, eventID).
		Group("attendance").Scan(&rows)

	out := map[string]int{"yes": 0, "no": 0, "maybe": 0, "pending": 0, "total": 0}
	for _, rw := range rows {
		out[string(rw.Attendance)] += rw.Count
		out["total"] += rw.Count
	}

	// Dynamic breakdowns for every "*_title" answer field (e.g. attendance_title,
	// spouse_attendance_title) grouped by their actual values, so the UI can show
	// clickable Member/Spouse Attendance cards without hardcoding the values.
	type titleRow struct {
		Key   string
		Value string
		Count int
	}
	var trows []titleRow
	a.DB.Raw(`
		SELECT je.key AS key, je.value AS value, count(*) AS count
		FROM rsvp_responses r
		CROSS JOIN LATERAL jsonb_each_text(r.answers) je
		WHERE r.organization_id = ? AND r.rsvp_event_id = ? AND r.deleted_at IS NULL
		  AND right(je.key, 6) = '_title' AND je.value <> ''
		GROUP BY je.key, je.value`, orgID, eventID).Scan(&trows)

	breakdowns := map[string]map[string]int{}
	for _, tr := range trows {
		if breakdowns[tr.Key] == nil {
			breakdowns[tr.Key] = map[string]int{}
		}
		breakdowns[tr.Key][tr.Value] += tr.Count
	}

	attendanceField := "attendance"
	if ev.AttendanceField != "" {
		attendanceField = ev.AttendanceField
	}
	var responses []models.RSVPResponse
	if err := a.DB.Select("attendance", "answers").
		Where("organization_id = ? AND rsvp_event_id = ?", orgID, eventID).Find(&responses).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to tally RSVP responses", nil, "")
	}
	contributors := ev.HeadcountContributors
	if len(contributors) == 0 {
		contributors = legacyHeadcountContributors(ev.AttendanceField)
	}
	spouseAttendanceKey := deriveSpouseAttendanceKey(contributors, ev.AttendanceField)
	attendanceBreakdown := buildRSVPAttendanceBreakdownWithKey(responses, ev.SpouseMobileField, spouseAttendanceKey)

	contributorTallies, totalAttending := buildRSVPHeadcount(responses, contributors)

	return r.SendEnvelope(map[string]interface{}{
		"yes": out["yes"], "no": out["no"], "maybe": out["maybe"],
		"pending": out["pending"], "total": out["total"],
		"breakdowns":        breakdowns,
		"attendance_field":  attendanceField,
		"member_attendance": attendanceBreakdown.Member,
		"spouse_attendance": attendanceBreakdown.Spouse,
		"contributors":      contributorTallies,
		"total_attending":   totalAttending,
	})
}

// ---------------------------------------------------------------------------
// Invite send (seed pending + best-effort template send)
// ---------------------------------------------------------------------------

type sendInvitesRequest struct {
	ContactIDs []string `json:"contact_ids"`
}

func (a *App) SendRSVPInvites(r *fastglue.Request) error {
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
	var req sendInvitesRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	requested, sent, skipped, failed := len(req.ContactIDs), 0, 0, 0
	errors := []map[string]string{}
	for _, cidStr := range req.ContactIDs {
		cid, perr := uuid.Parse(cidStr)
		if perr != nil {
			skipped++
			continue
		}
		var contact models.Contact
		if err := a.DB.Where("id = ? AND organization_id = ?", cid, orgID).First(&contact).Error; err != nil {
			skipped++
			continue
		}
		a.seedPendingRSVPResponse(orgID, event, contact.ID, contact.PhoneNumber, models.RSVPGuestSourceAPI)
		var response models.RSVPResponse
		if err := a.DB.Where("rsvp_event_id = ? AND contact_id = ?", event.ID, contact.ID).First(&response).Error; err != nil {
			failed++
			continue
		}
		if response.RespondedAt != nil {
			skipped++
			continue
		}
		// Sending is best-effort: a send failure must not fail seeding.
		if event.TemplateID != nil && event.WhatsAppAccount != "" {
			messageID, sendErr := a.sendRSVPInviteTemplate(event, event.TemplateID, &contact)
			if sendErr != nil {
				failed++
				errors = append(errors, map[string]string{"contact_id": contact.ID.String(), "error": sendErr.Error()})
				continue
			}
			now := time.Now().UTC()
			a.DB.Model(&response).Updates(map[string]interface{}{"invite_sent_at": now, "invite_message_id": messageID})
			sent++
		} else {
			failed++
			errors = append(errors, map[string]string{"contact_id": contact.ID.String(), "error": "invite template or WhatsApp account is not configured"})
		}
	}
	return r.SendEnvelope(map[string]interface{}{"requested": requested, "sent": sent, "skipped": skipped, "failed": failed, "errors": errors})
}

// sendRSVPInviteTemplate sends the given template to one contact using the WhatsApp client.
// Best-effort: logs and returns on any misconfiguration or send error.
func (a *App) sendRSVPInviteTemplate(event *models.RSVPEvent, templateID *uuid.UUID, contact *models.Contact) (string, error) {
	return a.sendRSVPTemplateWithParams(event, templateID, contact, nil)
}

func (a *App) sendRSVPTemplateWithParams(event *models.RSVPEvent, templateID *uuid.UUID, contact *models.Contact, templateParams map[string]string) (string, error) {
	if templateID == nil {
		return "", fmt.Errorf("template is not configured")
	}
	var account models.WhatsAppAccount
	if err := a.DB.Where("organization_id = ? AND name = ?", event.OrganizationID, event.WhatsAppAccount).
		First(&account).Error; err != nil {
		a.Log.Error("RSVP invite: account not found", "account", event.WhatsAppAccount, "error", err)
		return "", fmt.Errorf("account not found: %w", err)
	}
	var template models.Template
	if err := a.DB.Where("id = ? AND organization_id = ?", *templateID, event.OrganizationID).
		First(&template).Error; err != nil {
		a.Log.Error("RSVP invite: template not found", "error", err)
		return "", fmt.Errorf("template not found: %w", err)
	}
	if a.WhatsApp == nil {
		return "", fmt.Errorf("WhatsApp client is unavailable")
	}
	if err := validateRSVPReminderParams(&template, templateParams); err != nil {
		return "", err
	}
	// Build template components the same way campaign/template sends do, and honor the
	// account's delivery route (marketing-lite vs standard).
	bodyParams := templateutil.ResolveParamsMapFromMap(templateutil.ExtParamNames(template.BodyContent), templateParams)
	buttonParams, _ := templateutil.ResolveURLButtonParamsFromMap(template.Buttons, templateParams)
	components := whatsapp.BuildTemplateComponentsWithQuickReplyPayloads(bodyParams, buttonParams, nil, template.Buttons, template.HeaderType, "", "")
	waAccount := a.toWhatsAppAccount(&account)
	var messageID string
	var err error
	if models.ResolveTemplateDeliveryRoute(&account, &template) == models.TemplateDeliveryRouteMarketingMessagesLite {
		messageID, err = a.WhatsApp.SendMarketingTemplateMessage(context.Background(), waAccount, contact.PhoneNumber, template.Name, template.Language, components)
	} else {
		messageID, err = a.WhatsApp.SendTemplateMessage(context.Background(), waAccount, contact.PhoneNumber, template.Name, template.Language, components)
	}
	if err != nil {
		a.Log.Error("RSVP invite send failed", "error", err)
		return "", err
	}
	return messageID, nil
}

// ---------------------------------------------------------------------------
// XLSX export
// ---------------------------------------------------------------------------

func (a *App) ExportRSVPResponses(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	eventID, err := parsePathUUID(r, "id", "RSVP event")
	if err != nil {
		return nil
	}
	var event models.RSVPEvent
	if err := a.DB.Where("id = ? AND organization_id = ?", eventID, orgID).First(&event).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "RSVP event not found", nil, "")
	}
	var rows []models.RSVPResponse
	a.DB.Where("organization_id = ? AND rsvp_event_id = ?", orgID, eventID).
		Preload("Contact").Order("responded_at DESC NULLS LAST").Find(&rows)

	// Union of dynamic answer keys for stable columns.
	keySet := map[string]struct{}{}
	for _, row := range rows {
		for k := range row.Answers {
			keySet[k] = struct{}{}
		}
	}
	answerKeys := make([]string, 0, len(keySet))
	for k := range keySet {
		answerKeys = append(answerKeys, k)
	}
	sort.Strings(answerKeys)

	f := excelize.NewFile()
	sheet := "Responses"
	f.SetSheetName(f.GetSheetName(0), sheet)

	headers := append([]string{"Name", "Mobile", "Attendance", "Responded At (IST)"}, answerKeys...)
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}
	ist := time.FixedZone("IST", 5*3600+30*60) // UTC+5:30, no DST
	for rIdx, row := range rows {
		respondedAt := ""
		if row.RespondedAt != nil {
			respondedAt = row.RespondedAt.In(ist).Format("02/01/2006 15:04") // dd/mm/yyyy HH:mm IST
		}
		name := ""
		if row.Contact != nil {
			name = row.Contact.ProfileName
		}
		vals := []interface{}{name, row.PhoneNumber, string(row.Attendance), respondedAt}
		for _, k := range answerKeys {
			vals = append(vals, row.Answers[k])
		}
		for cIdx, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(cIdx+1, rIdx+2)
			f.SetCellValue(sheet, cell, v)
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to build workbook", nil, "")
	}
	r.RequestCtx.Response.Header.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	r.RequestCtx.Response.Header.Set("Content-Disposition", `attachment; filename="rsvp-responses.xlsx"`)
	r.RequestCtx.SetBody(buf.Bytes())
	return nil
}
