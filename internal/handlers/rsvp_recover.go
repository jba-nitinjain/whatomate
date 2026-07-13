package handlers

import (
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"

	"github.com/nikyjain/whatomate/internal/models"
)

// sessionHasAnswers reports whether a session holds at least one real answer
// (any key not prefixed with '_'), so we don't stamp truly-empty sessions as
// responded during recovery.
func sessionHasAnswers(s *models.ChatbotSession) bool {
	for k := range s.SessionData {
		if len(k) > 0 && k[0] != '_' {
			return true
		}
	}
	return false
}

// RecoverRSVPPartials commits partial answers left in abandoned chatbot sessions
// into RSVPResponses. Guests who answered some questions before the incremental
// save existed (or dropped off mid-flow) show as pending even though their
// answers were captured in the session; this finalizes those sessions so their
// partial data surfaces in the results. Idempotent.
func (a *App) RecoverRSVPPartials(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	eventID, err := parsePathUUID(r, "id", "RSVP event")
	if err != nil {
		return nil
	}

	var sessions []models.ChatbotSession
	if err := a.DB.
		Where("organization_id = ? AND session_data->>? = ?", orgID, rsvpEventIDKey, eventID.String()).
		Order("last_activity_at ASC").
		Find(&sessions).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to load sessions", nil, "")
	}

	recovered := 0
	for i := range sessions {
		if !sessionHasAnswers(&sessions[i]) {
			continue
		}
		a.finalizeRSVPFromSession(&sessions[i])
		recovered++
	}

	return r.SendEnvelope(map[string]interface{}{"recovered": recovered})
}
