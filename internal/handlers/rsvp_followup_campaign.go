package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

// rsvpFollowUpSendRequest is the body SendRSVPFollowUp decodes. Unlike
// reminders (which target explicit response_ids), a follow-up primarily
// targets an audience (mirrors PreviewRSVPFollowUp's query params); response_ids
// is an optional refinement letting the caller send to only a subset of the
// audience it already previewed. There is no staging_mime_type field - the
// reminder path removed trust in a client-supplied mime type
// (rsvp_reminder_media.go: loadStagedRSVPReminderMedia derives it from the
// staged file itself), so this does not reintroduce it.
type rsvpFollowUpSendRequest struct {
	Audience        string            `json:"audience"`
	AnswerKey       string            `json:"answer_key"`
	FlowID          string            `json:"flow_id"`
	TemplateID      string            `json:"template_id"`
	TemplateParams  map[string]string `json:"template_params"`
	StagingID       string            `json:"staging_id"`
	StagingFilename string            `json:"staging_filename"`
	ResponseIDs     []string          `json:"response_ids"`
}

type rsvpFollowUpCampaignResult struct {
	Campaign *models.BulkMessageCampaign
	Queued   int
	Skipped  []rsvpFollowUpSkip
}

// rsvpFollowUpGuestLoadErrorEnvelope classifies an error from
// loadRSVPFollowUpGuests for the send path. Mirrors
// rsvpFollowUpPreviewErrorEnvelope (rsvp_followup.go) with a send-appropriate
// message: an audience-clause validation error is safe to show verbatim as a
// 400, everything else (a Find timeout, connection drop) stays a generic 500.
func rsvpFollowUpGuestLoadErrorEnvelope(err error) (status int, message string) {
	var userErr rsvpUserFacingError
	if errors.As(err, &userErr) {
		return fasthttp.StatusBadRequest, userErr.Error()
	}
	return fasthttp.StatusInternalServerError, "Failed to load follow-up recipients"
}

// rsvpFollowUpCampaignErrorEnvelope classifies an error returned from
// createRSVPFollowUpCampaign into the (status, message) pair SendRSVPFollowUp
// should send. Mirrors rsvpReminderCampaignErrorEnvelope (rsvp_reminders.go):
// a rsvpUserFacingError - missing/expired staged media, or the
// validateCampaignReadyForStart backstop - carries a message safe to show the
// user, so it surfaces as a 400 with that exact text. Everything else (DB
// write failures, "campaign queue is unavailable") stays a generic 500.
func rsvpFollowUpCampaignErrorEnvelope(err error) (status int, message string) {
	var userErr rsvpUserFacingError
	if errors.As(err, &userErr) {
		return fasthttp.StatusBadRequest, userErr.Error()
	}
	return fasthttp.StatusInternalServerError, "Failed to create follow-up campaign"
}

func rsvpFollowUpCampaignName(eventName string, now time.Time) string {
	name := fmt.Sprintf("RSVP Follow-up - %s - %s", strings.TrimSpace(eventName), now.UTC().Format("2006-01-02 15:04 UTC"))
	runes := []rune(name)
	if len(runes) > 255 {
		name = string(runes[:255])
	}
	return name
}

// filterRSVPFollowUpRowsByResponseID narrows a loaded follow-up roster down to
// the requested response ids, when the caller supplied any. No response_ids
// at all means "use the whole audience" - the intended default, since the UI
// never has to send this field for an untouched selection. But once the
// caller DOES supply response_ids, this must not fail open: a stale or
// garbled selection (every id unparsable, or every id valid but none of them
// present in the loaded roster) previously fell back to returning `rows`
// untouched - silently sending to the entire audience instead of the chosen
// few, the opposite of what the caller asked for. Both cases now return a
// rsvpUserFacingError instead, which SendRSVPFollowUp surfaces as a 400
// (mirrors the rsvpUserFacingError contract documented in rsvp_reminders.go).
func filterRSVPFollowUpRowsByResponseID(rows []rsvpGuestRosterRow, responseIDs []string) ([]rsvpGuestRosterRow, error) {
	if len(responseIDs) == 0 {
		return rows, nil
	}
	ids, _ := parseRSVPResponseIDs(responseIDs)
	if len(ids) == 0 {
		return nil, rsvpUserFacingError{fmt.Errorf("selected guests could not be recognized - refresh the recipient list and try again")}
	}
	want := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	filtered := make([]rsvpGuestRosterRow, 0, len(rows))
	for _, row := range rows {
		if want[row.ID] {
			filtered = append(filtered, row)
		}
	}
	if len(filtered) == 0 {
		return nil, rsvpUserFacingError{fmt.Errorf("selected guests are no longer part of this audience - refresh the recipient list and try again")}
	}
	return filtered, nil
}

// rsvpFollowUpFlowIsEventPrimary reports whether flowID is the event's own
// primary RSVP flow. Running that flow as a follow-up would re-ask
// attendance and, via the RSVP merge (rsvp_flow.go), produce a confusing
// half-update rather than answering the one extra question a follow-up
// exists for - so SendRSVPFollowUp rejects it up front.
func rsvpFollowUpFlowIsEventPrimary(event *models.RSVPEvent, flowID uuid.UUID) bool {
	return event.FlowID != nil && *event.FlowID == flowID
}

// rsvpFollowUpFlowWrongAccount reports whether a follow-up flow's WhatsApp
// account does not match the event's. chatbot_processor.go treats
// ChatbotFlow.WhatsAppAccount as a hard gate against the account a message
// arrives on (matchFlowTrigger and startFlow's callers only ever consider a
// flow for the account the message came in on), so a follow-up flow scoped
// to a different account would send fine here and then silently fail to
// start the moment the guest taps through. An empty WhatsAppAccount on the
// flow (an org-level default) is not restricted to any one account, so it
// never mismatches.
func rsvpFollowUpFlowWrongAccount(event *models.RSVPEvent, flow *models.ChatbotFlow) bool {
	return flow.WhatsAppAccount != "" && flow.WhatsAppAccount != event.WhatsAppAccount
}

// SendRSVPFollowUp sends a follow-up campaign to the audience an admin
// configured, asking one extra question of guests who already responded. It
// mirrors createRSVPReminderCampaign's shape closely, including the lesson of
// the 15/07/2026 incident: media is attached and the campaign is validated
// with validateCampaignReadyForStart BEFORE anything is persisted, so a
// media-header template with no staged file cannot leave behind a committed
// campaign whose recipients stay pending forever.
func (a *App) SendRSVPFollowUp(r *fastglue.Request) error {
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

	var req rsvpFollowUpSendRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	audience := RSVPFollowUpAudience(strings.TrimSpace(req.Audience))
	answerKey := strings.TrimSpace(req.AnswerKey)
	if _, _, err := rsvpFollowUpAudienceClause(audience, answerKey); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}

	templateID, err := uuid.Parse(strings.TrimSpace(req.TemplateID))
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "template_id is required", nil, "")
	}
	var template models.Template
	if err := a.DB.Where("id = ? AND organization_id = ? AND whats_app_account = ?", templateID, orgID, event.WhatsAppAccount).
		First(&template).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "follow-up template was not found for this WhatsApp account", nil, "")
	}
	if !strings.EqualFold(template.Status, "APPROVED") {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "follow-up template must be approved", nil, "")
	}
	// Clean, user-fixable 400 for the common case (a media-header template
	// with no staged file) before any DB work below. This does not replace
	// validateCampaignReadyForStart inside createRSVPFollowUpCampaign - that
	// remains the authoritative backstop - it only gives a nicer message for
	// the common path, the same split SendRSVPReminders uses.
	if err := rsvpReminderMediaValidationError(&template, req.StagingID); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}

	flowID, err := uuid.Parse(strings.TrimSpace(req.FlowID))
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "flow_id is required", nil, "")
	}
	var flow models.ChatbotFlow
	if err := a.DB.Where("id = ? AND organization_id = ?", flowID, orgID).First(&flow).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "follow-up flow was not found", nil, "")
	}
	// Running the event's own primary flow as a follow-up would re-ask
	// attendance and, via the RSVP merge (rsvp_flow.go), produce a confusing
	// half-update rather than answering the one extra question a follow-up
	// exists for.
	if rsvpFollowUpFlowIsEventPrimary(event, flow.ID) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "follow-up cannot use the event's primary RSVP flow", nil, "")
	}
	if rsvpFollowUpFlowWrongAccount(event, &flow) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "follow-up flow must belong to the event's WhatsApp account", nil, "")
	}

	rows, err := a.loadRSVPFollowUpGuests(orgID, eventID, audience, answerKey)
	if err != nil {
		status, message := rsvpFollowUpGuestLoadErrorEnvelope(err)
		return r.SendErrorEnvelope(status, message, nil, "")
	}
	rows, err = filterRSVPFollowUpRowsByResponseID(rows, req.ResponseIDs)
	if err != nil {
		status, message := rsvpFollowUpGuestLoadErrorEnvelope(err)
		return r.SendErrorEnvelope(status, message, nil, "")
	}

	baseURL := a.requestPublicBaseURL(r.RequestCtx)
	result, err := a.createRSVPFollowUpCampaign(r.RequestCtx, event, &template, flow.ID, req.TemplateParams, rows, userID, req.StagingID, req.StagingFilename, baseURL)
	if err != nil {
		a.Log.Error("Failed to create RSVP follow-up campaign", "event_id", event.ID, "error", err)
		status, message := rsvpFollowUpCampaignErrorEnvelope(err)
		return r.SendErrorEnvelope(status, message, nil, "")
	}

	skipped := result.Skipped
	if skipped == nil {
		skipped = []rsvpFollowUpSkip{}
	}
	resp := map[string]interface{}{
		"requested": len(rows),
		"queued":    result.Queued,
		"skipped":   skipped,
	}
	if result.Campaign != nil {
		resp["campaign_id"] = result.Campaign.ID
		resp["campaign_name"] = result.Campaign.Name
	}
	return r.SendEnvelope(resp)
}

// createRSVPFollowUpCampaign snapshots the currently eligible follow-up
// audience into a linked campaign. It mirrors createRSVPReminderCampaign
// (rsvp_reminder_campaign.go) closely: media is attached and the campaign is
// validated (validateCampaignReadyForStart) BEFORE the DB transaction, so a
// campaign that cannot send is never persisted. FlowID is recorded on the
// campaign so the chatbot hook that handles a guest tapping the follow-up
// message knows which flow to run.
func (a *App) createRSVPFollowUpCampaign(
	ctx context.Context,
	event *models.RSVPEvent,
	template *models.Template,
	flowID uuid.UUID,
	templateParams map[string]string,
	rows []rsvpGuestRosterRow,
	createdBy uuid.UUID,
	stagingID, stagingFilename, baseURL string,
) (rsvpFollowUpCampaignResult, error) {
	result := rsvpFollowUpCampaignResult{}
	if len(rows) == 0 {
		return result, nil
	}
	if a.Queue == nil {
		return result, fmt.Errorf("campaign queue is unavailable")
	}

	// Same dedupe + skip predicate the preview uses (Task 4), so send cannot
	// queue more or fewer recipients than the preview promised.
	recipientRows, skipped := rsvpFollowUpEligibility(rows)
	result.Skipped = skipped
	if len(recipientRows) == 0 {
		return result, nil
	}

	now := time.Now().UTC()
	// The campaign's ID is minted here, before any row exists, so that media
	// attach (below) and validateCampaignReadyForStart can run - and can fail
	// - before anything is written to the DB. campaign.Template is
	// intentionally left unset (rather than assigned from the template
	// param) so tx.Create below does not try to auto-save/upsert the
	// associated Template row; validateCampaignReadyForStart loads it itself
	// from TemplateID+OrganizationID.
	flowIDCopy := flowID
	campaign := models.BulkMessageCampaign{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  event.OrganizationID,
		WhatsAppAccount: event.WhatsAppAccount,
		Name:            rsvpFollowUpCampaignName(event.Name, now),
		TemplateID:      template.ID,
		FlowID:          &flowIDCopy,
		Status:          models.CampaignStatusDraft,
		TotalRecipients: len(recipientRows),
		CreatedBy:       createdBy,
		SourceType:      models.CampaignSourceRSVPFollowUp,
		SourceID:        &event.ID,
	}

	recipients := make([]models.BulkMessageRecipient, 0, len(recipientRows))
	for i := range recipientRows {
		row := &recipientRows[i]
		resolved := resolveRSVPReminderParams(templateParams, event, &row.RSVPResponse)
		recipientName := rsvpReminderRowName(&row.RSVPResponse)
		if recipientName == "" {
			recipientName = row.PhoneNumber
		}
		recipients = append(recipients, models.BulkMessageRecipient{
			BaseModel:      models.BaseModel{ID: uuid.New()},
			CampaignID:     campaign.ID,
			PhoneNumber:    row.PhoneNumber,
			RecipientName:  recipientName,
			TemplateParams: rsvpReminderParamsJSON(resolved),
			Status:         models.MessageStatusPending,
		})
	}

	// Attach media and validate BEFORE writing anything to the DB. This is
	// the whole lesson of the 1008-failure incident on 15/07/2026: a
	// media-header template with no attachment must fail here, with nothing
	// persisted, rather than leaving a committed campaign whose recipients
	// stay "pending" forever.
	if stagingID != "" {
		if err := a.promoteRSVPReminderStagedMedia(&campaign, stagingID, stagingFilename, baseURL); err != nil {
			return result, err
		}
	}

	// The RSVP path calls enqueueCampaignRecipients directly and so never
	// passes through StartCampaign's gate (campaigns.go). Without this, a
	// media-header template fails once per recipient with Meta error 132012
	// while the campaign reports "completed".
	if err := a.validateCampaignReadyForStart(&campaign); err != nil {
		return result, rsvpUserFacingError{err}
	}

	err := a.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&campaign).Error; err != nil {
			return err
		}
		if err := tx.CreateInBatches(&recipients, campaignRecipientCreateBatchSize).Error; err != nil {
			return err
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
