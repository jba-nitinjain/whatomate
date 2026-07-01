package handlers

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/xuri/excelize/v2"
	"github.com/zerodha/fastglue"

	"github.com/nikyjain/whatomate/internal/models"
)

// ---------------------------------------------------------------------------
// Request types + helpers
// ---------------------------------------------------------------------------

type rsvpEventRequest struct {
	Name               string       `json:"name"`
	Description        string       `json:"description"`
	EventDate          *time.Time   `json:"event_date"`
	RSVPCloseAt        *time.Time   `json:"rsvp_close_at"`
	WhatsAppAccount    string       `json:"whatsapp_account"`
	FlowID             *string      `json:"flow_id"`
	Keyword            string       `json:"keyword"`
	AttendanceField    string       `json:"attendance_field"`
	AttendanceMap      models.JSONB `json:"attendance_map"`
	TemplateID         *string      `json:"template_id"`
	ReminderEnabled    bool         `json:"reminder_enabled"`
	ReminderAt         *time.Time   `json:"reminder_at"`
	ReminderTemplateID *string      `json:"reminder_template_id"`
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

func (a *App) applyRSVPEventRequest(e *models.RSVPEvent, req rsvpEventRequest) bool {
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
	e.ReminderEnabled = req.ReminderEnabled
	e.ReminderAt = req.ReminderAt
	if fid, ok := parseOptionalUUID(req.FlowID); ok {
		e.FlowID = fid
	} else {
		return false
	}
	if tid, ok := parseOptionalUUID(req.TemplateID); ok {
		e.TemplateID = tid
	} else {
		return false
	}
	if rid, ok := parseOptionalUUID(req.ReminderTemplateID); ok {
		e.ReminderTemplateID = rid
	} else {
		return false
	}
	return true
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
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  orgID,
		Status:          models.RSVPEventStatusDraft,
		AttendanceField: "attendance",
		CreatedBy:       userID,
	}
	if !a.applyRSVPEventRequest(&event, req) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid UUID field", nil, "")
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
	if !a.applyRSVPEventRequest(event, req) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid UUID field", nil, "")
	}
	if err := a.DB.Save(event).Error; err != nil {
		a.Log.Error("Failed to update rsvp event", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to update RSVP event", nil, "")
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
		Where("organization_id = ? AND rsvp_event_id = ?", orgID, eventID)
	if status := string(r.RequestCtx.QueryArgs().Peek("attendance")); status != "" {
		q = q.Where("attendance = ?", status)
	}
	var total int64
	q.Count(&total)
	var rows []models.RSVPResponse
	if err := pg.Apply(q.Preload("Contact").Order("responded_at DESC NULLS LAST")).Find(&rows).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to list responses", nil, "")
	}
	return r.SendEnvelope(map[string]interface{}{"responses": rows, "total": total, "page": pg.Page, "limit": pg.Limit})
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
	return r.SendEnvelope(out)
}

// ---------------------------------------------------------------------------
// Invite send (seed pending + best-effort template send)
// ---------------------------------------------------------------------------

type sendInvitesRequest struct {
	ContactIDs []string `json:"contact_ids"`
}

func (a *App) SendRSVPInvites(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
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

	seeded := 0
	for _, cidStr := range req.ContactIDs {
		cid, perr := uuid.Parse(cidStr)
		if perr != nil {
			continue
		}
		var contact models.Contact
		if err := a.DB.Where("id = ? AND organization_id = ?", cid, orgID).First(&contact).Error; err != nil {
			continue
		}
		a.seedPendingRSVPResponse(orgID, event, contact.ID, contact.PhoneNumber)
		seeded++
		// Sending is best-effort: a send failure must not fail seeding.
		if event.TemplateID != nil && event.WhatsAppAccount != "" {
			a.sendRSVPInviteTemplate(event, event.TemplateID, &contact)
		}
	}
	return r.SendEnvelope(map[string]interface{}{"seeded": seeded})
}

// sendRSVPInviteTemplate sends the given template to one contact using the WhatsApp client.
// Best-effort: logs and returns on any misconfiguration or send error.
func (a *App) sendRSVPInviteTemplate(event *models.RSVPEvent, templateID *uuid.UUID, contact *models.Contact) {
	if templateID == nil {
		return
	}
	var account models.WhatsAppAccount
	if err := a.DB.Where("organization_id = ? AND name = ?", event.OrganizationID, event.WhatsAppAccount).
		First(&account).Error; err != nil {
		a.Log.Error("RSVP invite: account not found", "account", event.WhatsAppAccount, "error", err)
		return
	}
	var template models.Template
	if err := a.DB.Where("id = ? AND organization_id = ?", *templateID, event.OrganizationID).
		First(&template).Error; err != nil {
		a.Log.Error("RSVP invite: template not found", "error", err)
		return
	}
	if a.WhatsApp == nil {
		return
	}
	if _, err := a.WhatsApp.SendTemplateMessage(context.Background(), a.toWhatsAppAccount(&account), contact.PhoneNumber, template.Name, template.Language, nil); err != nil {
		a.Log.Error("RSVP invite send failed", "error", err)
	}
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

	headers := append([]string{"Phone", "Attendance", "Responded At"}, answerKeys...)
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}
	for rIdx, row := range rows {
		respondedAt := ""
		if row.RespondedAt != nil {
			respondedAt = row.RespondedAt.Format("02/01/2006") // dd/mm/yyyy
		}
		vals := []interface{}{row.PhoneNumber, string(row.Attendance), respondedAt}
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
