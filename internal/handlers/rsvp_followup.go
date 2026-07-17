package handlers

import (
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// rsvpFollowUpSkip records a guest a follow-up would not message, and why.
// Same shape as rsvpReminderSkip (rsvp_reminder_campaign.go) so the admin sees
// a consistent skip report across reminders and follow-ups.
type rsvpFollowUpSkip struct {
	ResponseID uuid.UUID `json:"response_id"`
	Name       string    `json:"name"`
	Phone      string    `json:"phone"`
	Reason     string    `json:"reason"`
}

// loadRSVPFollowUpGuests loads the roster of guests matching a follow-up
// audience, org/event-scoped, in the same rsvpGuestRosterRow shape
// ListRSVPGuests uses (rsvp_guests.go:133) so the guest list and the
// follow-up preview/send agree on what a "guest row" is. The audience clause
// itself comes from rsvpFollowUpAudienceClause (rsvp_followup_audience.go),
// which binds answerKey rather than interpolating it - that contract is
// preserved here by passing it straight through as a bind arg.
func (a *App) loadRSVPFollowUpGuests(orgID, eventID uuid.UUID, audience RSVPFollowUpAudience, answerKey string) ([]rsvpGuestRosterRow, error) {
	clause, args, err := rsvpFollowUpAudienceClause(audience, answerKey)
	if err != nil {
		// Bad/missing answer_key or an unknown audience: the caller supplied
		// this, and the message (e.g. "missing_answer requires an answer
		// key") is safe and useful to show them verbatim. Wrap it so
		// PreviewRSVPFollowUp can tell it apart, via errors.As, from the
		// Find error below - a genuine DB failure the user cannot act on.
		return nil, rsvpUserFacingError{err}
	}
	q := a.DB.Where("rsvp_responses.organization_id = ? AND rsvp_responses.rsvp_event_id = ?", orgID, eventID).
		Where(clause, args...)
	var responses []models.RSVPResponse
	if err := q.Preload("Contact").Order("rsvp_responses.created_at DESC").Find(&responses).Error; err != nil {
		// Left unwrapped: a Find failure (DB timeout, connection drop) is an
		// infrastructure error, not something the user can fix, so it must
		// not be classified as user-facing by rsvpFollowUpPreviewErrorEnvelope.
		return nil, err
	}
	rows := make([]rsvpGuestRosterRow, 0, len(responses))
	for i := range responses {
		rows = append(rows, rsvpGuestRosterRow{
			RSVPResponse:  responses[i],
			JourneyStatus: rsvpFollowUpJourneyStatus(&responses[i]),
		})
	}
	return rows, nil
}

// rsvpFollowUpJourneyStatus mirrors the CASE expression ListRSVPGuests runs
// in SQL (rsvp_guests.go:150), so a follow-up guest row reports the same
// journey_status an admin sees on the guest list.
func rsvpFollowUpJourneyStatus(r *models.RSVPResponse) string {
	switch {
	case r.RespondedAt != nil:
		return "responded"
	case r.RSVPStartedAt != nil:
		return "in_progress"
	default:
		return "not_started"
	}
}

// rsvpFollowUpEligibility applies the exact predicate and dedupe the reminder
// send path uses (rsvpReminderSkipReason, dedupeRSVPReminderRows) to a loaded
// follow-up roster. Reusing them here - rather than writing a second
// predicate - is what keeps this preview and Task 5's send from being able to
// drift apart the way reminder preview and send once did (nil-contact rows
// counted eligible in preview, then dropped silently by send).
func rsvpFollowUpEligibility(rows []rsvpGuestRosterRow) (recipients []rsvpGuestRosterRow, skipped []rsvpFollowUpSkip) {
	kept, dropped := dedupeRSVPReminderRows(rows, func(r rsvpGuestRosterRow) string { return r.PhoneNumber })
	for _, dup := range dropped {
		skipped = append(skipped, rsvpFollowUpSkip{
			ResponseID: dup.ID,
			Name:       rsvpReminderRowName(&dup.RSVPResponse),
			Phone:      dup.PhoneNumber,
			Reason:     "duplicate phone number",
		})
	}
	for _, row := range kept {
		if reason := rsvpReminderSkipReason(row.Contact != nil, row.PhoneNumber); reason != "" {
			skipped = append(skipped, rsvpFollowUpSkip{
				ResponseID: row.ID,
				Name:       rsvpReminderRowName(&row.RSVPResponse),
				Phone:      row.PhoneNumber,
				Reason:     reason,
			})
			continue
		}
		recipients = append(recipients, row)
	}
	return recipients, skipped
}

// rsvpFollowUpPreviewErrorEnvelope classifies an error from
// loadRSVPFollowUpGuests into the (status, message) pair PreviewRSVPFollowUp
// sends. Mirrors rsvpReminderCampaignErrorEnvelope (rsvp_reminders.go): an
// audience-clause validation error (bad/missing answer_key, unknown
// audience) is wrapped in rsvpUserFacingError at rsvpFollowUpAudienceClause's
// call site and is safe to show verbatim as a 400. Everything else - a Find
// timeout, connection drop - is an infrastructure failure the user cannot
// act on, so it stays a generic 500 rather than leaking driver-level text.
func rsvpFollowUpPreviewErrorEnvelope(err error) (status int, message string) {
	var userErr rsvpUserFacingError
	if errors.As(err, &userErr) {
		return fasthttp.StatusBadRequest, userErr.Error()
	}
	return fasthttp.StatusInternalServerError, "Failed to preview follow-up"
}

// PreviewRSVPFollowUp reports who an audience filter would message and who
// it would skip and why, using the exact loader and eligibility predicate
// Task 5's send reuses - so this preview cannot promise more recipients than
// send will actually queue.
func (a *App) PreviewRSVPFollowUp(r *fastglue.Request) error {
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
	audience := RSVPFollowUpAudience(strings.TrimSpace(string(r.RequestCtx.QueryArgs().Peek("audience"))))
	answerKey := strings.TrimSpace(string(r.RequestCtx.QueryArgs().Peek("answer_key")))
	rows, err := a.loadRSVPFollowUpGuests(orgID, eventID, audience, answerKey)
	if err != nil {
		status, msg := rsvpFollowUpPreviewErrorEnvelope(err)
		return r.SendErrorEnvelope(status, msg, nil, "")
	}
	recipients, skipped := rsvpFollowUpEligibility(rows)
	if skipped == nil {
		skipped = []rsvpFollowUpSkip{}
	}
	return r.SendEnvelope(map[string]interface{}{
		"eligible":   len(recipients),
		"skipped":    skipped,
		"recipients": recipients,
	})
}
