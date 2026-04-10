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
	if msg := validateChatRepairApplyRequest(req, candidates); msg != "" {
		return r.SendErrorEnvelope(fasthttp.StatusConflict, msg, nil, "")
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

func validateChatRepairApplyRequest(req ChatRepairApplyRequest, candidates []ChatRepairCandidate) string {
	if len(req.ContactIDs) == 0 && len(req.ManualMergeContactIDs) == 0 {
		return ""
	}

	candidateByID := make(map[string]ChatRepairCandidate, len(candidates))
	for _, candidate := range candidates {
		candidateByID[candidate.ContactID] = candidate
	}

	for _, rawID := range req.ContactIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}

		candidate, ok := candidateByID[id]
		if !ok {
			return "One or more selected chat repairs no longer exist. Refresh candidates and try again."
		}
		if candidate.Action == chatRepairActionMove {
			continue
		}
		if candidate.Action == chatRepairActionConflict {
			return "One or more selected chat repairs are no longer safe to move because they conflict with another chat that resolves to the same target phone number. Refresh candidates and resolve the conflict before retrying."
		}
		if candidate.Action == chatRepairActionMergeRequired {
			return "One or more selected chat repairs now require a manual merge. Refresh candidates and review them before retrying."
		}
		return "Selected chat repairs are no longer safe to apply. Refresh candidates and try again."
	}

	for _, rawID := range req.ManualMergeContactIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}

		candidate, ok := candidateByID[id]
		if !ok {
			return "One or more selected chat repairs no longer exist. Refresh candidates and try again."
		}
		if candidate.Action == chatRepairActionMergeRequired {
			continue
		}
		if candidate.Action == chatRepairActionConflict {
			return "One or more selected chat repairs cannot be merged automatically because their target is ambiguous. Refresh candidates and resolve the conflict before retrying."
		}
		if candidate.Action == chatRepairActionMove {
			return "One or more selected chat repairs no longer require a manual merge. Refresh candidates and try again."
		}
		return "Selected chat repairs are no longer safe to apply. Refresh candidates and try again."
	}

	return ""
}
