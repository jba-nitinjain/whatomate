package handlers

import (
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

// ListLeads lists leads with pagination and filtering
func (a *App) ListLeads(r *fastglue.Request) error {
	orgID, _, ok := a.leadAuth(r, models.ActionRead)
	if !ok {
		return nil
	}

	query := a.DB.Model(&models.Lead{}).Where("leads.organization_id = ?", orgID)
	args := r.RequestCtx.QueryArgs()
	if sID, err := uuid.Parse(string(args.Peek("source_id"))); err == nil {
		query = query.Where("lead_source_id = ?", sID)
	}
	if formID := string(args.Peek("form_id")); formID != "" {
		query = query.Where("form_id = ?", formID)
	}
	if status := string(args.Peek("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if phone := string(args.Peek("phone")); phone != "" {
		query = query.Where("(phone LIKE ? OR phone_raw LIKE ?)", "%"+phone+"%", "%"+phone+"%")
	}
	if fromTime, err := time.Parse(time.RFC3339, string(args.Peek("from"))); err == nil {
		query = query.Where("lead_created_at >= ?", fromTime)
	}
	if toTime, err := time.Parse(time.RFC3339, string(args.Peek("to"))); err == nil {
		query = query.Where("lead_created_at <= ?", toTime)
	}

	var total int64
	query.Count(&total)

	pg := parsePaginationWithDefaults(r, 20, 100)
	var leads []models.Lead
	if err := pg.Apply(query.Preload("LeadSource").Preload("Mapping").Preload("Contact").Preload("Message").
		Order("lead_created_at DESC, created_at DESC")).Find(&leads).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to fetch leads", nil, "")
	}

	return r.SendEnvelope(map[string]interface{}{
		"leads": leads,
		"total": total,
		"page":  pg.Page,
		"limit": pg.Limit,
	})
}

// loadLead fetches an org-scoped lead by the {id} path param; nil means an envelope was sent.
func (a *App) loadLead(r *fastglue.Request, orgID uuid.UUID, preload bool) *models.Lead {
	leadID, err := parsePathUUID(r, "id", "lead")
	if err != nil {
		return nil
	}
	db := a.DB
	if preload {
		db = db.Preload("LeadSource").Preload("Mapping").Preload("Contact").Preload("Message")
	}
	lead, err := findByIDAndOrg[models.Lead](db, r, leadID, orgID, "Lead")
	if err != nil {
		return nil
	}
	return lead
}

// GetLead retrieves a full lead detail
func (a *App) GetLead(r *fastglue.Request) error {
	orgID, _, ok := a.leadAuth(r, models.ActionRead)
	if !ok {
		return nil
	}
	lead := a.loadLead(r, orgID, true)
	if lead == nil {
		return nil
	}
	return r.SendEnvelope(lead)
}

// requeueLead puts a lead back on the processor queue. When forceResend is
// true the previous message link is cleared so the template is sent again.
func (a *App) requeueLead(lead *models.Lead, forceResend bool) error {
	now := time.Now()
	status := models.StatusLeadReceived
	if len(lead.FieldData) > 0 {
		status = models.StatusLeadFetched
	}
	updates := map[string]interface{}{
		"status":          status,
		"next_attempt_at": now,
		"last_error":      "",
		"attempts":        0,
	}
	if forceResend {
		updates["message_id"] = nil
		updates["sent_at"] = nil
	}
	if err := a.DB.Model(&models.Lead{}).Where("id = ? AND organization_id = ?", lead.ID, lead.OrganizationID).
		Updates(updates).Error; err != nil {
		return err
	}
	lead.Status = status
	lead.NextAttemptAt = &now
	lead.LastError = ""
	lead.Attempts = 0
	if forceResend {
		lead.MessageID = nil
		lead.SentAt = nil
	}
	if a.LeadProcessor != nil {
		a.LeadProcessor.Kick()
	}
	return nil
}

// RetryLead manually re-queues a lead for processing. A lead that already has
// a delivered message is not messaged again (use SendLeadNow for that).
func (a *App) RetryLead(r *fastglue.Request) error {
	orgID, _, ok := a.leadAuth(r, models.ActionExecute)
	if !ok {
		return nil
	}
	lead := a.loadLead(r, orgID, false)
	if lead == nil {
		return nil
	}
	if err := a.requeueLead(lead, false); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to retry lead", nil, "")
	}
	return r.SendEnvelope(lead)
}

// SendLeadNow queues an explicit (re)send of the mapped template for a lead.
// The background processor performs the send, so the request never blocks on
// Graph/WhatsApp calls and the processor's double-send guard still applies.
func (a *App) SendLeadNow(r *fastglue.Request) error {
	orgID, _, ok := a.leadAuth(r, models.ActionExecute)
	if !ok {
		return nil
	}
	lead := a.loadLead(r, orgID, false)
	if lead == nil {
		return nil
	}
	if lead.Status == models.StatusLeadSending {
		return r.SendErrorEnvelope(fasthttp.StatusConflict, "Lead is currently being sent", nil, "")
	}
	if err := a.requeueLead(lead, true); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to queue lead", nil, "")
	}
	return r.SendEnvelope(lead)
}

// DeleteLead soft-deletes a lead or purges PII if purge=true
func (a *App) DeleteLead(r *fastglue.Request) error {
	orgID, _, ok := a.leadAuth(r, models.ActionDelete)
	if !ok {
		return nil
	}
	lead := a.loadLead(r, orgID, false)
	if lead == nil {
		return nil
	}

	if string(r.RequestCtx.QueryArgs().Peek("purge")) != "true" {
		if err := a.DB.Delete(lead).Error; err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to delete lead", nil, "")
		}
		return r.SendEnvelope(map[string]bool{"success": true})
	}

	// Hard-delete the lead and remove metadata.lead from the linked contact.
	err := a.DB.Transaction(func(tx *gorm.DB) error {
		if lead.ContactID != nil {
			if err := tx.Model(&models.Contact{}).
				Where("id = ? AND organization_id = ?", *lead.ContactID, orgID).
				Update("metadata", gorm.Expr("COALESCE(metadata, '{}'::jsonb) - 'lead'")).Error; err != nil {
				return err
			}
		}
		return tx.Unscoped().Delete(lead).Error
	})
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to purge lead", nil, "")
	}
	return r.SendEnvelope(map[string]bool{"success": true})
}

// GetLeadStats returns status metrics (today and total) for the top strip
func (a *App) GetLeadStats(r *fastglue.Request) error {
	orgID, _, ok := a.leadAuth(r, models.ActionRead)
	if !ok {
		return nil
	}

	type statusCount struct {
		Status string `json:"status"`
		Count  int64  `json:"count"`
	}
	startOfToday := time.Now().Truncate(24 * time.Hour)

	var todayCounts []statusCount
	a.DB.Model(&models.Lead{}).
		Select("status, count(*) as count").
		Where("organization_id = ? AND lead_created_at >= ?", orgID, startOfToday).
		Group("status").
		Scan(&todayCounts)

	var totalCounts []statusCount
	a.DB.Model(&models.Lead{}).
		Select("status, count(*) as count").
		Where("organization_id = ?", orgID).
		Group("status").
		Scan(&totalCounts)

	return r.SendEnvelope(map[string]interface{}{
		"today": todayCounts,
		"total": totalCounts,
	})
}
