package handlers

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

const (
	chatRepairNoSelectionMessage      = "No chat repair candidates selected"
	chatRepairSafeSelectionStaleError = "Selected safe chat repair candidates are stale or no longer safe. Re-scan and try again."
	chatRepairMergeSelectionStaleError = "Selected merge chat repair candidates are stale or no longer merge candidates. Re-scan and try again."
)

// ApplyChatRepairCandidates applies the selected repair candidates after validating them against the latest scan state.
func (a *App) ApplyChatRepairCandidates(r *fastglue.Request) error {
	userID, ok := r.RequestCtx.UserValue("user_id").(uuid.UUID)
	if !ok {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	if !a.IsSuperAdmin(userID) {
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, "Only super admins can access chat repair", nil, "")
	}

	var req ChatRepairApplyRequest
	if err := json.Unmarshal(r.RequestCtx.Request.Body(), &req); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid request body", nil, "")
	}

	candidates, _, err := a.buildChatRepairCandidates(0)
	if err != nil {
		a.Log.Error("Failed to load chat repair candidates before apply", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to load chat repair candidates", nil, "")
	}

	if statusCode, message, valid := validateChatRepairApplyRequest(req, candidates); !valid {
		return r.SendErrorEnvelope(statusCode, message, nil, "")
	}

	selection := newChatRepairSelection(req.ContactIDs, req.ManualMergeContactIDs)
	result, err := a.executeChatRepairSelection(candidates, selection)
	if err != nil {
		a.Log.Error("Failed to apply chat repair", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to apply chat repair", nil, "")
	}

	return r.SendEnvelope(result)
}

// ScanChatRepairCandidates scans current repair candidates and auto-applies every safe move candidate in one transaction.
func (a *App) ScanChatRepairCandidates(r *fastglue.Request) error {
	userID, ok := r.RequestCtx.UserValue("user_id").(uuid.UUID)
	if !ok {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	if !a.IsSuperAdmin(userID) {
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, "Only super admins can access chat repair", nil, "")
	}

	candidates, summary, err := a.buildChatRepairCandidates(0)
	if err != nil {
		a.Log.Error("Failed to scan chat repair candidates", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to scan chat repair candidates", nil, "")
	}

	selection := buildSafeChatRepairSelection(candidates)
	if len(selection.Selected) == 0 {
		return r.SendEnvelope(ChatRepairScanResult{
			AutoApplied: ChatRepairApplyResult{},
			Summary:     summary,
			Candidates:  candidates,
		})
	}

	result, err := a.executeChatRepairSelection(candidates, selection)
	if err != nil {
		a.Log.Error("Failed to auto-apply safe chat repair", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to auto-apply safe chat repair", nil, "")
	}

	refreshedCandidates, refreshedSummary, err := a.buildChatRepairCandidates(0)
	if err != nil {
		a.Log.Error("Failed to refresh chat repair candidates after scan", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to refresh chat repair candidates", nil, "")
	}

	return r.SendEnvelope(ChatRepairScanResult{
		AutoApplied: result,
		Summary:     refreshedSummary,
		Candidates:  refreshedCandidates,
	})
}

func validateChatRepairApplyRequest(req ChatRepairApplyRequest, candidates []ChatRepairCandidate) (int, string, bool) {
	selection := newChatRepairSelection(req.ContactIDs, req.ManualMergeContactIDs)
	if len(selection.Selected) == 0 && len(selection.ManualMergeSelected) == 0 {
		return fasthttp.StatusBadRequest, chatRepairNoSelectionMessage, false
	}

	candidatesByID := make(map[string]ChatRepairCandidate, len(candidates))
	for _, candidate := range candidates {
		candidatesByID[candidate.ContactID] = candidate
	}

	for contactID := range selection.Selected {
		if selection.ManualMergeSelected[contactID] {
			return fasthttp.StatusBadRequest, "A chat repair candidate cannot be selected for both move and manual merge", false
		}

		candidate, ok := candidatesByID[contactID]
		if !ok {
			return fasthttp.StatusConflict, chatRepairSafeSelectionStaleError, false
		}
		if candidate.Action != chatRepairActionMove {
			if candidate.Reason != "" {
				return fasthttp.StatusConflict, candidate.Reason, false
			}
			return fasthttp.StatusConflict, chatRepairSafeSelectionStaleError, false
		}
	}

	for contactID := range selection.ManualMergeSelected {
		candidate, ok := candidatesByID[contactID]
		if !ok {
			return fasthttp.StatusConflict, chatRepairMergeSelectionStaleError, false
		}
		if candidate.Action != chatRepairActionMergeRequired {
			if candidate.Reason != "" {
				return fasthttp.StatusConflict, candidate.Reason, false
			}
			return fasthttp.StatusConflict, chatRepairMergeSelectionStaleError, false
		}
	}

	return 0, "", true
}
