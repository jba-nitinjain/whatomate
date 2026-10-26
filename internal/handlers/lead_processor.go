package handlers

import (
	"context"
	"time"

	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/internal/websocket"
)

const (
	// leadBatchSize bounds how many leads one claim leases at a time; small so
	// the lease cannot expire before the batch is worked through.
	leadBatchSize = 10
	// leadLeaseSQL is the lease length applied on claim and renewed per lead.
	leadLeaseSQL = "NOW() + INTERVAL '5 minutes'"
	// leadMaxAttempts is the attempt count after which retryable errors fail permanently.
	leadMaxAttempts = 5
)

// claimableLeadStatuses are the statuses the worker picks up. "sending" is
// deliberately absent: a lead that crashed mid-send is never re-sent
// automatically and must be re-queued by an operator.
var claimableLeadStatuses = []string{models.StatusLeadReceived, models.StatusLeadFetched, models.StatusLeadRetry}

// LeadProcessor is a background worker that fetches leads from Meta Graph API,
// creates/updates contacts, resolves form mappings, and sends template messages.
type LeadProcessor struct {
	app      *App
	interval time.Duration
	kick     chan struct{}
	stopCh   chan struct{}
}

// NewLeadProcessor creates a new LeadProcessor instance
func NewLeadProcessor(app *App, interval time.Duration) *LeadProcessor {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	return &LeadProcessor{
		app:      app,
		interval: interval,
		kick:     make(chan struct{}, 1),
		stopCh:   make(chan struct{}),
	}
}

// Start runs the processor loop until ctx is cancelled or Stop is called.
func (p *LeadProcessor) Start(ctx context.Context) {
	p.app.Log.Info("Lead processor started", "interval", p.interval)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	p.processBatch(ctx)
	for {
		select {
		case <-ctx.Done():
			p.app.Log.Info("Lead processor stopped by context")
			return
		case <-p.stopCh:
			p.app.Log.Info("Lead processor stopped")
			return
		case <-p.kick:
			p.processBatch(ctx)
		case <-ticker.C:
			p.processBatch(ctx)
		}
	}
}

// Kick triggers an immediate processing run without waiting for ticker
func (p *LeadProcessor) Kick() {
	select {
	case p.kick <- struct{}{}:
	default:
	}
}

// Stop terminates the lead processor
func (p *LeadProcessor) Stop() {
	select {
	case <-p.stopCh:
	default:
		close(p.stopCh)
	}
}

// processBatch claims a batch of due leads with SKIP LOCKED and processes them.
func (p *LeadProcessor) processBatch(ctx context.Context) {
	var leads []models.Lead
	err := p.app.DB.Raw(`
		UPDATE leads
		SET attempts = attempts + 1,
		    next_attempt_at = `+leadLeaseSQL+`
		WHERE id IN (
			SELECT id FROM leads
			WHERE status IN ?
			  AND next_attempt_at <= NOW()
			  AND deleted_at IS NULL
			ORDER BY next_attempt_at ASC
			LIMIT ?
			FOR UPDATE SKIP LOCKED
		)
		RETURNING *`, claimableLeadStatuses, leadBatchSize).Scan(&leads).Error
	if err != nil {
		p.app.Log.Error("Failed to claim leads batch", "error", err)
		return
	}

	for i := range leads {
		if ctx.Err() != nil {
			return
		}
		if !p.renewLease(&leads[i]) {
			continue
		}
		p.processLead(ctx, &leads[i])
	}
}

// renewLease extends the claim on a lead right before it is worked on. It
// returns false when the lead was changed elsewhere (deleted, sent, or
// re-queued) since the batch was claimed.
func (p *LeadProcessor) renewLease(lead *models.Lead) bool {
	res := p.app.DB.Exec(`UPDATE leads SET next_attempt_at = `+leadLeaseSQL+`
		WHERE id = ? AND status = ? AND deleted_at IS NULL`, lead.ID, lead.Status)
	return res.Error == nil && res.RowsAffected == 1
}

// saveLead persists the processor-owned columns of a lead. Rows that are
// already "sending" or "sent" are never overwritten, so a stale worker cannot
// roll back a send performed by another worker.
func (p *LeadProcessor) saveLead(lead *models.Lead) {
	err := p.app.DB.Model(&models.Lead{}).
		Where("id = ? AND status NOT IN ?", lead.ID, []string{models.StatusLeadSending, models.StatusLeadSent}).
		Updates(map[string]interface{}{
			"status":          lead.Status,
			"next_attempt_at": lead.NextAttemptAt,
			"attempts":        lead.Attempts,
			"last_error":      lead.LastError,
			"field_data":      lead.FieldData,
			"form_id":         lead.FormID,
			"ad_id":           lead.AdID,
			"ad_name":         lead.AdName,
			"campaign_name":   lead.CampaignName,
			"platform":        lead.Platform,
			"is_organic":      lead.IsOrganic,
			"mapping_id":      lead.MappingID,
			"name":            lead.Name,
			"phone_raw":       lead.PhoneRaw,
			"phone":           lead.Phone,
			"email":           lead.Email,
			"contact_id":      lead.ContactID,
		}).Error
	if err != nil {
		p.app.Log.Error("Failed to save lead", "lead_id", lead.ID, "error", err)
	}
}

// finish records a terminal or deferred outcome and notifies the UI.
func (p *LeadProcessor) finish(lead *models.Lead, status, lastError string) {
	lead.Status = status
	lead.LastError = lastError
	p.saveLead(lead)
	p.broadcastLeadUpdate(lead)
}

// failOrRetry schedules a retry for retryable errors, otherwise fails the lead.
func (p *LeadProcessor) failOrRetry(lead *models.Lead, retryable bool, reason string) {
	if retryable && lead.Attempts < leadMaxAttempts {
		next := time.Now().Add(backoffDuration(lead.Attempts))
		lead.NextAttemptAt = &next
		p.finish(lead, models.StatusLeadRetry, reason)
		return
	}
	p.finish(lead, models.StatusLeadFailed, reason)
}

func (p *LeadProcessor) broadcastLeadUpdate(lead *models.Lead) {
	if p.app.WSHub != nil {
		p.app.WSHub.BroadcastToOrg(lead.OrganizationID, websocket.WSMessage{
			Type:    websocket.TypeLeadUpdate,
			Payload: lead,
		})
	}
}

func backoffDuration(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 30 * time.Second
	case 2:
		return 2 * time.Minute
	case 3:
		return 10 * time.Minute
	case 4:
		return 1 * time.Hour
	default:
		return 6 * time.Hour
	}
}
