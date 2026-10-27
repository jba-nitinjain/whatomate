package handlers

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

// ChatbotSessionResponse is a chatbot session plus display-friendly extras.
// The embedded model keeps every field the endpoint returned before.
type ChatbotSessionResponse struct {
	models.ChatbotSession
	FlowName     string `json:"flow_name"`
	MessageCount int64  `json:"message_count"`
}

// ListChatbotSessions lists chatbot sessions (conversation history).
// Query params: status, flow_id, search (contact name/phone), from, to
// (YYYY-MM-DD, applied to started_at), page, limit.
func (a *App) ListChatbotSessions(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceFlowsChatbot, models.ActionRead)
	if err != nil {
		return nil
	}

	args := r.RequestCtx.QueryArgs()
	query := a.DB.Model(&models.ChatbotSession{}).Where("organization_id = ?", orgID)

	if status := string(args.Peek("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if v := string(args.Peek("flow_id")); v != "" {
		flowID, perr := uuid.Parse(v)
		if perr != nil {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid flow_id", nil, "")
		}
		query = query.Where("current_flow_id = ?", flowID)
	}
	if search := strings.TrimSpace(string(args.Peek("search"))); search != "" {
		pattern := "%" + search + "%"
		query = query.Where(
			"phone_number ILIKE ? OR contact_id IN (SELECT id FROM contacts WHERE organization_id = ? AND (profile_name ILIKE ? OR phone_number ILIKE ?))",
			pattern, orgID, pattern, pattern,
		)
	}
	if from, ok := parseDateParam(r, "from"); ok {
		query = query.Where("started_at >= ?", from)
	}
	if to, ok := parseDateParam(r, "to"); ok {
		query = query.Where("started_at < ?", to.Add(24*time.Hour))
	}

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		a.Log.Error("Failed to count sessions", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to fetch sessions", nil, "")
	}

	// Default/max 100 keeps the pre-pagination behaviour for callers that send no params.
	pg := parsePaginationWithDefaults(r, 100, 100)
	var sessions []models.ChatbotSession
	if err := pg.Apply(query.Preload("Contact").Order("last_activity_at DESC")).Find(&sessions).Error; err != nil {
		a.Log.Error("Failed to fetch sessions", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to fetch sessions", nil, "")
	}

	return r.SendEnvelope(listEnvelope("sessions", a.buildSessionResponses(orgID, sessions), total, pg))
}

// GetChatbotSession gets a single chatbot session with its message log.
func (a *App) GetChatbotSession(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceFlowsChatbot, models.ActionRead)
	if err != nil {
		return nil
	}

	id, err := parsePathUUID(r, "id", "session")
	if err != nil {
		return nil
	}

	var session models.ChatbotSession
	if err := a.DB.Where("id = ? AND organization_id = ?", id, orgID).
		Preload("Contact").
		Preload("Messages", func(db *gorm.DB) *gorm.DB { return db.Order("created_at ASC") }).
		First(&session).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Session not found", nil, "")
	}

	resp := a.buildSessionResponses(orgID, []models.ChatbotSession{session})[0]
	resp.MessageCount = int64(len(session.Messages))
	return r.SendEnvelope(resp)
}

// buildSessionResponses attaches flow names and message counts to sessions.
func (a *App) buildSessionResponses(orgID uuid.UUID, sessions []models.ChatbotSession) []ChatbotSessionResponse {
	flowIDs := make([]uuid.UUID, 0)
	sessionIDs := make([]uuid.UUID, 0, len(sessions))
	for _, s := range sessions {
		sessionIDs = append(sessionIDs, s.ID)
		if s.CurrentFlowID != nil {
			flowIDs = append(flowIDs, *s.CurrentFlowID)
		}
	}

	flowNames := map[uuid.UUID]string{}
	if len(flowIDs) > 0 {
		var flows []models.ChatbotFlow
		// Unscoped so deleted flows still show their name in history.
		a.DB.Unscoped().Select("id", "name").
			Where("organization_id = ? AND id IN ?", orgID, flowIDs).Find(&flows)
		for _, f := range flows {
			flowNames[f.ID] = f.Name
		}
	}

	counts := map[uuid.UUID]int64{}
	if len(sessionIDs) > 0 {
		var rows []struct {
			SessionID uuid.UUID
			Count     int64
		}
		a.DB.Model(&models.ChatbotSessionMessage{}).
			Select("session_id, COUNT(*) AS count").
			Where("session_id IN ?", sessionIDs).
			Group("session_id").Scan(&rows)
		for _, row := range rows {
			counts[row.SessionID] = row.Count
		}
	}

	out := make([]ChatbotSessionResponse, len(sessions))
	for i, s := range sessions {
		name := ""
		if s.CurrentFlowID != nil {
			name = flowNames[*s.CurrentFlowID]
		}
		if name == "" {
			if n, ok := s.SessionData["_flow_name"].(string); ok {
				name = n
			}
		}
		out[i] = ChatbotSessionResponse{ChatbotSession: s, FlowName: name, MessageCount: counts[s.ID]}
	}
	return out
}
