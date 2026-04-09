package handlers

import (
	"strconv"
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
	summary := ChatRepairSummary{ScannedContacts: int64(len(rows))}

	orgNames, err := a.lookupOrganizationNames(rows)
	if err != nil {
		return nil, ChatRepairSummary{}, err
	}

	for _, row := range rows {
		candidate := ChatRepairCandidate{
			ContactID:            row.ContactID,
			PhoneNumber:          row.PhoneNumber,
			ProfileName:          row.ProfileName,
			CurrentOrgID:         row.CurrentOrgID,
			CurrentOrgName:       orgNames[row.CurrentOrgID],
			CurrentAccount:       row.CurrentAccount,
			TargetOrgID:          row.TargetOrgID,
			TargetOrgName:        orgNames[row.TargetOrgID],
			TargetAccount:        row.TargetAccount,
			AffectedMessageCount: row.AffectedMessageCount,
			LastMessageAt:        row.LastMessageAt,
			PhoneNumberID:        row.SamplePhoneNumberID,
		}

		switch {
		case row.TargetOrgCount != 1:
			candidate.Action = chatRepairActionConflict
			candidate.Reason = "Multiple organizations match the same phone_number_id"
		case row.TargetAccountCount != 1:
			candidate.Action = chatRepairActionConflict
			candidate.Reason = "Multiple WhatsApp accounts match the same phone_number_id"
		case row.CurrentOrgID == row.TargetOrgID && strings.TrimSpace(row.CurrentAccount) == strings.TrimSpace(row.TargetAccount):
			continue
		default:
			var targetContact models.Contact
			err := a.DB.Where("organization_id = ? AND phone_number = ? AND deleted_at IS NULL", row.TargetOrgID, row.PhoneNumber).First(&targetContact).Error
			if err == nil && targetContact.ID.String() != row.ContactID {
				candidate.Action = chatRepairActionMergeRequired
				candidate.TargetContactID = targetContact.ID.String()
				candidate.Reason = "A contact with this phone number already exists in the target organization"
			} else if err != nil && err != gorm.ErrRecordNotFound {
				return nil, ChatRepairSummary{}, err
			} else {
				candidate.Action = chatRepairActionMove
				candidate.Reason = "Safe to move this chat to the resolved organization/account"
			}
		}

		summary.AffectedExternalMessages += row.AffectedMessageCount
		switch candidate.Action {
		case chatRepairActionMove:
			summary.MoveCandidates++
			summary.AutoFixableCandidates++
		case chatRepairActionMergeRequired:
			summary.MergeRequiredCandidates++
		case chatRepairActionConflict:
			summary.ConflictCandidates++
		}

		candidates = append(candidates, candidate)
	}

	return candidates, summary, nil
}

func (a *App) loadChatRepairBaseRows(limit int) ([]chatRepairBaseRow, error) {
	query := `
		SELECT
			c.id::text AS contact_id,
			c.organization_id::text AS current_org_id,
			c.phone_number,
			c.profile_name,
			c.whats_app_account AS current_account,
			COUNT(DISTINCT wa.organization_id::text) AS target_org_count,
			COUNT(DISTINCT wa.name) AS target_account_count,
			MIN(wa.organization_id::text) AS target_org_id,
			MIN(wa.name) AS target_account,
			COUNT(DISTINCT m.id::text) AS affected_message_count,
			MAX(m.created_at) AS last_message_at,
			MIN(m.metadata->>'phone_number_id') AS sample_phone_number_id
		FROM contacts c
		JOIN messages m
			ON m.contact_id = c.id
			AND m.deleted_at IS NULL
		JOIN whatsapp_accounts wa
			ON wa.phone_id = m.metadata->>'phone_number_id'
			AND wa.deleted_at IS NULL
		WHERE c.deleted_at IS NULL
			AND m.metadata->>'source' = 'external_api'
			AND COALESCE(m.metadata->>'source_system', '') = 'aws_lambda'
			AND COALESCE(m.metadata->>'phone_number_id', '') <> ''
		GROUP BY c.id, c.organization_id, c.phone_number, c.profile_name, c.whats_app_account
		ORDER BY MAX(m.created_at) DESC
	`
	if limit > 0 {
		query += " LIMIT " + strconv.Itoa(limit)
	}

	var rows []chatRepairBaseRow
	return rows, a.DB.Raw(query).Scan(&rows).Error
}

func (a *App) lookupOrganizationNames(rows []chatRepairBaseRow) (map[string]string, error) {
	ids := make([]string, 0, len(rows)*2)
	seen := make(map[string]bool, len(rows)*2)
	for _, row := range rows {
		if row.CurrentOrgID != "" && !seen[row.CurrentOrgID] {
			ids = append(ids, row.CurrentOrgID)
			seen[row.CurrentOrgID] = true
		}
		if row.TargetOrgID != "" && !seen[row.TargetOrgID] {
			ids = append(ids, row.TargetOrgID)
			seen[row.TargetOrgID] = true
		}
	}
	if len(ids) == 0 {
		return map[string]string{}, nil
	}

	var orgs []models.Organization
	if err := a.DB.Select("id", "name").Where("id IN ?", ids).Find(&orgs).Error; err != nil {
		return nil, err
	}

	names := make(map[string]string, len(orgs))
	for _, org := range orgs {
		names[org.ID.String()] = org.Name
	}
	return names, nil
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
