package handlers

import (
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// PreviewChatRepairCandidates lists legacy external message chats that look mapped to the wrong org/account.
func (a *App) PreviewChatRepairCandidates(r *fastglue.Request) error {
	userID, ok := r.RequestCtx.UserValue("user_id").(uuid.UUID)
	if !ok {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	if !a.IsSuperAdmin(userID) {
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, "Only super admins can access chat repair", nil, "")
	}

	limit := 100
	if rawLimit := strings.TrimSpace(string(r.RequestCtx.QueryArgs().Peek("limit"))); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil {
			switch {
			case parsed <= 0:
				limit = 100
			case parsed > 500:
				limit = 500
			default:
				limit = parsed
			}
		}
	}

	candidates, summary, err := a.buildChatRepairCandidates(limit)
	if err != nil {
		a.Log.Error("Failed to preview chat repair candidates", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to scan chat repair candidates", nil, "")
	}

	return r.SendEnvelope(map[string]any{
		"summary":    summary,
		"candidates": candidates,
	})
}
