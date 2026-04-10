package handlers

import (
	"strings"

	"gorm.io/gorm"
)

type chatRepairSelection struct {
	Selected            map[string]bool
	ManualMergeSelected map[string]bool
}

func newChatRepairSelection(contactIDs, manualMergeContactIDs []string) chatRepairSelection {
	selection := chatRepairSelection{
		Selected:            make(map[string]bool, len(contactIDs)),
		ManualMergeSelected: make(map[string]bool, len(manualMergeContactIDs)),
	}

	for _, id := range contactIDs {
		trimmedID := strings.TrimSpace(id)
		if trimmedID != "" {
			selection.Selected[trimmedID] = true
		}
	}
	for _, id := range manualMergeContactIDs {
		trimmedID := strings.TrimSpace(id)
		if trimmedID != "" {
			selection.ManualMergeSelected[trimmedID] = true
		}
	}

	return selection
}

func buildSafeChatRepairSelection(candidates []ChatRepairCandidate) chatRepairSelection {
	contactIDs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Action == chatRepairActionMove {
			contactIDs = append(contactIDs, candidate.ContactID)
		}
	}

	return newChatRepairSelection(contactIDs, nil)
}

func (s chatRepairSelection) includes(candidate ChatRepairCandidate) bool {
	if len(s.Selected) == 0 && len(s.ManualMergeSelected) == 0 {
		return true
	}

	return s.Selected[candidate.ContactID] || s.ManualMergeSelected[candidate.ContactID]
}

func (a *App) executeChatRepairSelection(candidates []ChatRepairCandidate, selection chatRepairSelection) (ChatRepairApplyResult, error) {
	tx := a.DB.Begin()
	if tx.Error != nil {
		return ChatRepairApplyResult{}, tx.Error
	}

	result, err := a.applyChatRepairSelectionTx(tx, candidates, selection)
	if err != nil {
		tx.Rollback()
		return ChatRepairApplyResult{}, err
	}

	if err := tx.Commit().Error; err != nil {
		return ChatRepairApplyResult{}, err
	}

	return result, nil
}

func (a *App) applyChatRepairSelectionTx(tx *gorm.DB, candidates []ChatRepairCandidate, selection chatRepairSelection) (ChatRepairApplyResult, error) {
	result := ChatRepairApplyResult{}

	for _, candidate := range candidates {
		if !selection.includes(candidate) {
			continue
		}

		result.ProcessedCandidates++
		switch candidate.Action {
		case chatRepairActionMove:
			if !selection.Selected[candidate.ContactID] {
				result.SkippedCandidates++
				continue
			}

			updatedMessages, err := a.applyChatRepairMove(tx, candidate)
			if err != nil {
				return ChatRepairApplyResult{}, err
			}

			result.UpdatedContacts++
			result.UpdatedMessages += updatedMessages
		case chatRepairActionMergeRequired:
			if !selection.ManualMergeSelected[candidate.ContactID] {
				result.SkippedCandidates++
				continue
			}

			updatedMessages, err := a.applyChatRepairManualMerge(tx, candidate)
			if err != nil {
				return ChatRepairApplyResult{}, err
			}

			result.UpdatedContacts++
			result.UpdatedMessages += updatedMessages
		default:
			continue
		}
	}

	return result, nil
}
