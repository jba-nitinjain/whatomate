package handlers

import (
	"strings"
	"time"

	"github.com/shridarpatil/whatomate/internal/models"
	"gorm.io/gorm"
)

func (a *App) buildChatRepairCandidates(limit int) ([]ChatRepairCandidate, ChatRepairSummary, error) {
	rows, err := a.loadChatRepairBaseRows(limit)
	if err != nil {
		return nil, ChatRepairSummary{}, err
	}

	candidates := make([]ChatRepairCandidate, 0, len(rows))
	contactIDs := make([]string, 0, len(rows))

	orgNames, err := a.lookupOrganizationNames(rows)
	if err != nil {
		return nil, ChatRepairSummary{}, err
	}

	for _, row := range rows {
		contactIDs = append(contactIDs, row.ContactID)
	}

	sampleMessages, err := a.loadChatRepairSampleMessages(contactIDs, 3)
	if err != nil {
		return nil, ChatRepairSummary{}, err
	}
	messageLocations, err := a.loadChatRepairMessageLocations(contactIDs)
	if err != nil {
		return nil, ChatRepairSummary{}, err
	}

	for _, row := range rows {
		resolution, err := a.resolveChatRepairTarget(row.ContactID, row.CurrentAccount)
		if err != nil {
			return nil, ChatRepairSummary{}, err
		}
		if resolution.TargetOrgCount == 0 && resolution.TargetAccountCount == 0 {
			continue
		}
		if resolution.TargetOrgID != "" && orgNames[resolution.TargetOrgID] == "" {
			var targetOrg models.Organization
			if err := a.DB.Select("id", "name").Where("id = ?", resolution.TargetOrgID).First(&targetOrg).Error; err == nil {
				orgNames[targetOrg.ID.String()] = targetOrg.Name
			}
		}

		candidate := ChatRepairCandidate{
			ContactID:            row.ContactID,
			PhoneNumber:          row.PhoneNumber,
			ProfileName:          row.ProfileName,
			CurrentOrgID:         row.CurrentOrgID,
			CurrentOrgName:       orgNames[row.CurrentOrgID],
			CurrentAccount:       row.CurrentAccount,
			TargetOrgID:          resolution.TargetOrgID,
			TargetOrgName:        orgNames[resolution.TargetOrgID],
			TargetAccount:        resolution.TargetAccount,
			AffectedMessageCount: row.AffectedMessageCount,
			LastMessageAt:        row.LastMessageAt,
			PhoneNumberID:        resolution.PhoneNumberID,
			SampleMessages:       sampleMessages[row.ContactID],
		}
		currentMatchesTarget := row.CurrentOrgID == resolution.TargetOrgID &&
			strings.TrimSpace(row.CurrentAccount) == strings.TrimSpace(resolution.TargetAccount)
		messageDrift := hasChatRepairMessageDrift(messageLocations[row.ContactID], resolution.TargetOrgID, resolution.TargetAccount)

		switch {
		case resolution.TargetOrgCount != 1:
			candidate.Action = chatRepairActionConflict
			candidate.Reason = "Multiple organizations match the available contact/message routing evidence"
		case resolution.TargetAccountCount != 1:
			candidate.Action = chatRepairActionConflict
			candidate.Reason = "Multiple WhatsApp accounts match the available contact/message routing evidence"
		case currentMatchesTarget && !messageDrift:
			continue
		default:
			var targetContact models.Contact
			err := a.DB.Unscoped().Where("organization_id = ? AND phone_number = ?", resolution.TargetOrgID, row.PhoneNumber).First(&targetContact).Error
			if err == nil && targetContact.ID.String() != row.ContactID {
				candidate.Action = chatRepairActionMergeRequired
				candidate.TargetContactID = targetContact.ID.String()
				if targetContact.DeletedAt.Valid {
					candidate.Reason = "A matching deleted contact already exists in the target organization and must be restored before merging"
				} else {
					candidate.Reason = "A contact with this phone number already exists in the target organization"
				}
			} else if err != nil && err != gorm.ErrRecordNotFound {
				return nil, ChatRepairSummary{}, err
			} else {
				candidate.Action = chatRepairActionMove
				if currentMatchesTarget {
					candidate.Reason = "Safe to normalize misrouted messages under this chat to the resolved organization/account"
				} else {
					candidate.Reason = "Safe to move this chat to the resolved organization/account"
				}
			}
		}

		candidates = append(candidates, candidate)
	}

	candidates, summary := reconcileChatRepairCandidates(int64(len(rows)), candidates)
	return candidates, summary, nil
}

func (a *App) applyChatRepairMove(tx *gorm.DB, candidate ChatRepairCandidate) (int64, error) {
	now := time.Now().UTC()

	contactUpdate := tx.Model(&models.Contact{}).Where("id = ? AND deleted_at IS NULL", candidate.ContactID).Updates(map[string]any{
		"organization_id":   candidate.TargetOrgID,
		"whats_app_account": candidate.TargetAccount,
		"updated_at":        now,
	})
	if contactUpdate.Error != nil {
		return 0, contactUpdate.Error
	}

	messageUpdate := tx.Model(&models.Message{}).Where("contact_id = ? AND deleted_at IS NULL", candidate.ContactID).Updates(map[string]any{
		"organization_id":   candidate.TargetOrgID,
		"whats_app_account": candidate.TargetAccount,
		"updated_at":        now,
	})
	if messageUpdate.Error != nil {
		return 0, messageUpdate.Error
	}

	if err := a.refreshChatRepairContactSnapshot(tx, candidate.ContactID, now); err != nil {
		return 0, err
	}

	return messageUpdate.RowsAffected, nil
}

func (a *App) applyChatRepairManualMerge(tx *gorm.DB, candidate ChatRepairCandidate) (int64, error) {
	now := time.Now().UTC()

	if strings.TrimSpace(candidate.TargetContactID) == "" {
		return 0, gorm.ErrRecordNotFound
	}

	var targetContact models.Contact
	if err := tx.Unscoped().Where("id = ?", candidate.TargetContactID).First(&targetContact).Error; err != nil {
		return 0, err
	}
	if targetContact.DeletedAt.Valid {
		if err := tx.Unscoped().Model(&targetContact).Updates(map[string]any{
			"deleted_at": nil,
			"updated_at": now,
		}).Error; err != nil {
			return 0, err
		}
	}

	messageUpdate := tx.Model(&models.Message{}).Where("contact_id = ? AND deleted_at IS NULL", candidate.ContactID).Updates(map[string]any{
		"contact_id":         candidate.TargetContactID,
		"organization_id":    candidate.TargetOrgID,
		"whats_app_account":  candidate.TargetAccount,
		"updated_at":         now,
	})
	if messageUpdate.Error != nil {
		return 0, messageUpdate.Error
	}

	if err := a.refreshChatRepairContactSnapshot(tx, candidate.TargetContactID, now); err != nil {
		return 0, err
	}

	var remainingMessages int64
	if err := tx.Model(&models.Message{}).Where("contact_id = ? AND deleted_at IS NULL", candidate.ContactID).Count(&remainingMessages).Error; err != nil {
		return 0, err
	}

	if remainingMessages == 0 {
		if err := tx.Model(&models.Contact{}).Where("id = ? AND deleted_at IS NULL", candidate.ContactID).Updates(map[string]any{
			"deleted_at": now,
			"updated_at": now,
		}).Error; err != nil {
			return 0, err
		}
	} else if err := a.refreshChatRepairContactSnapshot(tx, candidate.ContactID, now); err != nil {
		return 0, err
	}

	return messageUpdate.RowsAffected, nil
}

func (a *App) refreshChatRepairContactSnapshot(tx *gorm.DB, contactID string, now time.Time) error {
	var latest models.Message
	if err := tx.Where("contact_id = ? AND deleted_at IS NULL", contactID).Order("created_at DESC").First(&latest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return tx.Model(&models.Contact{}).Where("id = ?", contactID).Updates(map[string]any{
				"last_message_at":      nil,
				"last_message_preview": "",
				"updated_at":           now,
			}).Error
		}
		return err
	}

	return tx.Model(&models.Contact{}).Where("id = ?", contactID).Updates(map[string]any{
		"last_message_at":      latest.CreatedAt,
		"last_message_preview": a.getPersistedMessagePreview(&latest),
		"whats_app_account":    latest.WhatsAppAccount,
		"updated_at":           now,
	}).Error
}
