package handlers

import (
	"fmt"
	"strings"
)

// RSVPFollowUpAudience selects which guests a follow-up targets.
type RSVPFollowUpAudience string

const (
	// RSVPFollowUpAudienceNotStarted matches guests who never began the RSVP.
	// Mirrors the journey definition at rsvp_guests.go:150.
	RSVPFollowUpAudienceNotStarted RSVPFollowUpAudience = "not_started"
	// RSVPFollowUpAudienceRespondedYes matches guests attending.
	RSVPFollowUpAudienceRespondedYes RSVPFollowUpAudience = "responded_yes"
	// RSVPFollowUpAudienceRespondedNo matches guests not attending.
	RSVPFollowUpAudienceRespondedNo RSVPFollowUpAudience = "responded_no"
	// RSVPFollowUpAudienceMissingAnswer matches guests who replied but never
	// answered a given question. Self-cleaning: as answers arrive, the audience
	// shrinks, so re-sending chases only whoever is still missing.
	RSVPFollowUpAudienceMissingAnswer RSVPFollowUpAudience = "missing_answer"
)

// rsvpFollowUpAudienceClause returns a WHERE fragment and its bind args for an
// audience. The answer key originates from user configuration and is always bound,
// never interpolated.
func rsvpFollowUpAudienceClause(audience RSVPFollowUpAudience, answerKey string) (string, []interface{}, error) {
	switch audience {
	case RSVPFollowUpAudienceNotStarted:
		return "rsvp_responses.rsvp_started_at IS NULL AND rsvp_responses.responded_at IS NULL", nil, nil

	case RSVPFollowUpAudienceRespondedYes:
		return "rsvp_responses.responded_at IS NOT NULL AND rsvp_responses.attendance = ?",
			[]interface{}{"yes"}, nil

	case RSVPFollowUpAudienceRespondedNo:
		return "rsvp_responses.responded_at IS NOT NULL AND rsvp_responses.attendance = ?",
			[]interface{}{"no"}, nil

	case RSVPFollowUpAudienceMissingAnswer:
		key := strings.TrimSpace(answerKey)
		if key == "" {
			return "", nil, fmt.Errorf("missing_answer requires an answer key")
		}
		// Scoped to responders: chasing someone who never replied is what Reminders
		// is for. NULLIF(x, '') IS NULL is true when the answer is absent (->> is
		// NULL) OR present-but-empty (NULLIF collapses '' to NULL) - either way it
		// counts as missing.
		return `rsvp_responses.responded_at IS NOT NULL
			AND NULLIF(rsvp_responses.answers ->> ?, '') IS NULL`,
			[]interface{}{key}, nil

	default:
		return "", nil, fmt.Errorf("unknown follow-up audience: %q", audience)
	}
}
