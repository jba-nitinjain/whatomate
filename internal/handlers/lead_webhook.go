package handlers

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm/clause"
)

// LeadgenWebhook represents Meta's leadgen webhook payload structure
type LeadgenWebhook struct {
	Object string                `json:"object"` // must be "page"
	Entry  []leadgenWebhookEntry `json:"entry"`
}

type leadgenWebhookEntry struct {
	ID      string `json:"id"` // page id
	Time    int64  `json:"time"`
	Changes []struct {
		Field string             `json:"field"` // "leadgen"
		Value leadgenChangeValue `json:"value"`
	} `json:"changes"`
}

type leadgenChangeValue struct {
	LeadgenID   string `json:"leadgen_id"`
	PageID      string `json:"page_id"`
	FormID      string `json:"form_id"`
	AdID        string `json:"ad_id"`
	AdgroupID   string `json:"adgroup_id"`
	CreatedTime int64  `json:"created_time"`
}

// LeadWebhookVerify handles GET /api/webhook/leadgen
func (a *App) LeadWebhookVerify(r *fastglue.Request) error {
	mode := string(r.RequestCtx.QueryArgs().Peek("hub.mode"))
	token := string(r.RequestCtx.QueryArgs().Peek("hub.verify_token"))
	challenge := string(r.RequestCtx.QueryArgs().Peek("hub.challenge"))

	if mode != "subscribe" || token == "" {
		a.Log.Warn("Lead webhook verification failed - invalid mode or token", "mode", mode)
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, "Verification failed", nil, "")
	}

	verified := token == a.Config.WhatsApp.WebhookVerifyToken
	if !verified {
		var source models.LeadSource
		verified = a.DB.Where("webhook_verify_token = ?", token).First(&source).Error == nil
	}
	if !verified {
		a.Log.Warn("Lead webhook verification failed - token not recognised")
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, "Verification failed", nil, "")
	}

	r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
	r.RequestCtx.SetBodyString(challenge)
	return nil
}

// LeadWebhookHandler handles POST /api/webhook/leadgen.
//
// Every entry is attributed to the LeadSource that owns its page, and the
// request signature is verified against the App Secret of each distinct
// source before anything is stored. A source without an App Secret can never
// accept webhooks (fail closed).
func (a *App) LeadWebhookHandler(r *fastglue.Request) error {
	body := r.RequestCtx.PostBody()

	var payload LeadgenWebhook
	if err := json.Unmarshal(body, &payload); err != nil {
		a.Log.Warn("Failed to parse leadgen webhook body", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid JSON payload", nil, "")
	}
	if payload.Object != "page" {
		a.Log.Warn("Rejected non-page webhook object", "object", payload.Object)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Expected object=page", nil, "")
	}

	sigHeader := r.RequestCtx.Request.Header.Peek("X-Hub-Signature-256")
	if len(sigHeader) == 0 {
		a.Log.Warn("Missing X-Hub-Signature-256 header on lead webhook")
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, "Missing signature", nil, "")
	}

	// Resolve and authenticate every page before inserting anything.
	sources := make(map[string]*models.LeadSource)
	verified := make(map[uuid.UUID]bool)
	for _, entry := range payload.Entry {
		if entry.ID == "" || sources[entry.ID] != nil {
			continue
		}
		source, err := a.getLeadSourceCached(entry.ID)
		if err != nil {
			// Unknown page: skip it; other entries may still be valid.
			a.Log.Warn("Leadgen webhook for unknown page ID", "page_id", entry.ID, "error", err)
			continue
		}
		if !verified[source.ID] {
			if source.AppSecret == "" {
				a.Log.Error("Lead source has no App Secret; rejecting leadgen webhook", "page_id", entry.ID, "source_id", source.ID)
				return r.SendErrorEnvelope(fasthttp.StatusForbidden, "Signature verification unavailable", nil, "")
			}
			if !verifyWebhookSignature(body, sigHeader, []byte(source.AppSecret)) {
				a.Log.Warn("Invalid X-Hub-Signature-256 on lead webhook", "page_id", entry.ID)
				return r.SendErrorEnvelope(fasthttp.StatusForbidden, "Invalid signature", nil, "")
			}
			verified[source.ID] = true
		}
		sources[entry.ID] = source
	}

	now := time.Now()
	insertedCount := 0
	for _, entry := range payload.Entry {
		source := sources[entry.ID]
		if source == nil {
			continue
		}
		for _, change := range entry.Changes {
			if change.Field != "leadgen" || change.Value.LeadgenID == "" {
				continue
			}
			if a.insertWebhookLead(source, change.Value, now) {
				insertedCount++
			}
		}
		if err := a.DB.Model(&models.LeadSource{}).Where("id = ?", source.ID).Update("last_event_at", now).Error; err != nil {
			a.Log.Error("Failed to update lead source last_event_at", "source_id", source.ID, "error", err)
		}
	}

	if insertedCount > 0 && a.LeadProcessor != nil {
		a.LeadProcessor.Kick()
	}
	r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
	return nil
}

// insertWebhookLead queues one leadgen event idempotently (ON CONFLICT leadgen_id DO NOTHING).
func (a *App) insertWebhookLead(source *models.LeadSource, val leadgenChangeValue, now time.Time) bool {
	leadCreatedAt := now
	if val.CreatedTime != 0 {
		leadCreatedAt = time.Unix(val.CreatedTime, 0)
	}
	lead := models.Lead{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: source.OrganizationID,
		LeadSourceID:   source.ID,
		LeadgenID:      val.LeadgenID,
		PageID:         source.PageID,
		FormID:         val.FormID,
		AdID:           val.AdID,
		Status:         models.StatusLeadReceived,
		NextAttemptAt:  &now,
		LeadCreatedAt:  leadCreatedAt,
	}
	res := a.DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "leadgen_id"}}, DoNothing: true}).Create(&lead)
	if res.Error != nil {
		a.Log.Error("Failed to insert leadgen webhook row", "leadgen_id", val.LeadgenID, "error", res.Error)
		return false
	}
	return res.RowsAffected > 0
}
