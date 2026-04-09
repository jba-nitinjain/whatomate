package handlers

import (
	"strconv"
	"strings"

	"github.com/shridarpatil/whatomate/internal/models"
)

type chatRepairResolutionRow struct {
	TargetOrgID   string `gorm:"column:target_org_id"`
	TargetAccount string `gorm:"column:target_account"`
	PhoneNumberID string `gorm:"column:phone_number_id"`
}

func (a *App) loadChatRepairBaseRows(limit int) ([]chatRepairBaseRow, error) {
	query := `
		SELECT
			c.id::text AS contact_id,
			c.organization_id::text AS current_org_id,
			c.phone_number,
			c.profile_name,
			c.whats_app_account AS current_account,
			COUNT(m.id) AS affected_message_count,
			MAX(m.created_at) AS last_message_at
		FROM contacts c
		JOIN messages m
			ON m.contact_id = c.id
			AND m.deleted_at IS NULL
		WHERE c.deleted_at IS NULL
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
	ids := make([]string, 0, len(rows))
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		if row.CurrentOrgID != "" && !seen[row.CurrentOrgID] {
			ids = append(ids, row.CurrentOrgID)
			seen[row.CurrentOrgID] = true
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

func (a *App) resolveChatRepairTarget(contactID, currentAccount string) (chatRepairTargetResolution, error) {
	phoneRows, err := a.loadChatRepairPhoneIDTargets(contactID)
	if err != nil {
		return chatRepairTargetResolution{}, err
	}
	if len(phoneRows) > 0 {
		return reduceChatRepairTargets(phoneRows), nil
	}

	accountRows, err := a.loadChatRepairAccountTargets(contactID, currentAccount)
	if err != nil {
		return chatRepairTargetResolution{}, err
	}
	return reduceChatRepairTargets(accountRows), nil
}

func (a *App) loadChatRepairPhoneIDTargets(contactID string) ([]chatRepairResolutionRow, error) {
	query := `
		SELECT DISTINCT
			wa.organization_id::text AS target_org_id,
			wa.name AS target_account,
			wa.phone_id AS phone_number_id
		FROM messages m
		JOIN whatsapp_accounts wa
			ON wa.phone_id = m.metadata->>'phone_number_id'
			AND wa.deleted_at IS NULL
		WHERE m.deleted_at IS NULL
			AND m.contact_id::text = ?
			AND COALESCE(m.metadata->>'phone_number_id', '') <> ''
	`

	var rows []chatRepairResolutionRow
	return rows, a.DB.Raw(query, contactID).Scan(&rows).Error
}

func (a *App) loadChatRepairAccountTargets(contactID, currentAccount string) ([]chatRepairResolutionRow, error) {
	query := `
		SELECT DISTINCT
			wa.organization_id::text AS target_org_id,
			wa.name AS target_account,
			wa.phone_id AS phone_number_id
		FROM whatsapp_accounts wa
		WHERE wa.deleted_at IS NULL
			AND wa.name IN (
				SELECT DISTINCT name
				FROM (
					SELECT NULLIF(TRIM(?), '') AS name
					UNION
					SELECT NULLIF(TRIM(m.whats_app_account), '') AS name
					FROM messages m
					WHERE m.deleted_at IS NULL
						AND m.contact_id::text = ?
				) names
				WHERE name IS NOT NULL
			)
	`

	var rows []chatRepairResolutionRow
	return rows, a.DB.Raw(query, currentAccount, contactID).Scan(&rows).Error
}

func reduceChatRepairTargets(rows []chatRepairResolutionRow) chatRepairTargetResolution {
	if len(rows) == 0 {
		return chatRepairTargetResolution{}
	}

	orgs := make(map[string]bool, len(rows))
	accounts := make(map[string]bool, len(rows))
	phoneIDs := make(map[string]bool, len(rows))
	resolution := chatRepairTargetResolution{}

	for _, row := range rows {
		if row.TargetOrgID != "" {
			orgs[row.TargetOrgID] = true
			if resolution.TargetOrgID == "" {
				resolution.TargetOrgID = row.TargetOrgID
			}
		}
		if row.TargetAccount != "" {
			accounts[row.TargetAccount] = true
			if resolution.TargetAccount == "" {
				resolution.TargetAccount = row.TargetAccount
			}
		}
		trimmedPhoneID := strings.TrimSpace(row.PhoneNumberID)
		if trimmedPhoneID != "" {
			phoneIDs[trimmedPhoneID] = true
			if resolution.PhoneNumberID == "" {
				resolution.PhoneNumberID = trimmedPhoneID
			}
		}
	}

	resolution.TargetOrgCount = int64(len(orgs))
	resolution.TargetAccountCount = int64(len(accounts))
	if len(phoneIDs) > 1 {
		resolution.PhoneNumberID = ""
	}

	return resolution
}
