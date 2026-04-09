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

// ApplyChatRepairCandidates applies safe move repairs and explicitly approved manual merges.
func (a *App) ApplyChatRepairCandidates(r *fastglue.Request) error {
	userID, ok := r.RequestCtx.UserValue("user_id").(uuid.UUID)
	if !ok {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	if !a.IsSuperAdmin(userID) {
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, "Only super admins can run chat repair", nil, "")
	}

	var req ChatRepairApplyRequest
	if len(r.RequestCtx.Request.Body()) > 0 {
		if err := a.decodeRequest(r, &req); err != nil {
			return nil
		}
	}

	candidates, _, err := a.buildChatRepairCandidates(0)
	if err != nil {
		a.Log.Error("Failed to scan chat repair candidates before applying fixes", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to scan chat repair candidates", nil, "")
	}

	selected := make(map[string]bool, len(req.ContactIDs))
	for _, id := range req.ContactIDs {
		selected[strings.TrimSpace(id)] = true
	}
	manualMergeSelected := make(map[string]bool, len(req.ManualMergeContactIDs))
	for _, id := range req.ManualMergeContactIDs {
		manualMergeSelected[strings.TrimSpace(id)] = true
	}

	tx := a.DB.Begin()
	if tx.Error != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to start chat repair", nil, "")
	}

	result := ChatRepairApplyResult{}
	for _, candidate := range candidates {
		if len(selected) > 0 || len(manualMergeSelected) > 0 {
			if !selected[candidate.ContactID] && !manualMergeSelected[candidate.ContactID] {
				continue
			}
		}

		result.ProcessedCandidates++
		switch candidate.Action {
		case chatRepairActionMove:
			if !selected[candidate.ContactID] {
				result.SkippedCandidates++
				continue
			}

			updatedMessages, err := a.applyChatRepairMove(tx, candidate)
			if err != nil {
				tx.Rollback()
				a.Log.Error("Failed to apply chat repair candidate", "error", err, "contact_id", candidate.ContactID)
				return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to apply chat repair", nil, "")
			}

			result.UpdatedContacts++
			result.UpdatedMessages += updatedMessages
		case chatRepairActionMergeRequired:
			if !manualMergeSelected[candidate.ContactID] {
				result.SkippedCandidates++
				continue
			}

			updatedMessages, err := a.applyChatRepairManualMerge(tx, candidate)
			if err != nil {
				tx.Rollback()
				a.Log.Error("Failed to apply manual chat repair merge", "error", err, "contact_id", candidate.ContactID, "target_contact_id", candidate.TargetContactID)
				return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to apply chat repair", nil, "")
			}

			result.UpdatedContacts++
			result.UpdatedMessages += updatedMessages
		default:
			continue
		}
	}

	if err := tx.Commit().Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to commit chat repair", nil, "")
	}

	return r.SendEnvelope(result)
}
