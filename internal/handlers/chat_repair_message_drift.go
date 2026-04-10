package handlers

import "strings"

type chatRepairMessageLocationRow struct {
	ContactID        string `gorm:"column:contact_id"`
	OrganizationID   string `gorm:"column:organization_id"`
	WhatsAppAccount  string `gorm:"column:whats_app_account"`
}

func (a *App) loadChatRepairMessageLocations(contactIDs []string) (map[string][]chatRepairMessageLocationRow, error) {
	if len(contactIDs) == 0 {
		return map[string][]chatRepairMessageLocationRow{}, nil
	}

	query := `
		SELECT DISTINCT
			m.contact_id::text AS contact_id,
			m.organization_id::text AS organization_id,
			m.whats_app_account
		FROM messages m
		WHERE m.deleted_at IS NULL
			AND m.contact_id::text IN ?
	`

	var rows []chatRepairMessageLocationRow
	if err := a.DB.Raw(query, contactIDs).Scan(&rows).Error; err != nil {
		return nil, err
	}

	locations := make(map[string][]chatRepairMessageLocationRow, len(contactIDs))
	for _, row := range rows {
		locations[row.ContactID] = append(locations[row.ContactID], row)
	}

	return locations, nil
}

func hasChatRepairMessageDrift(rows []chatRepairMessageLocationRow, targetOrgID, targetAccount string) bool {
	trimmedTargetAccount := strings.TrimSpace(targetAccount)
	for _, row := range rows {
		if row.OrganizationID != targetOrgID {
			return true
		}
		if strings.TrimSpace(row.WhatsAppAccount) != trimmedTargetAccount {
			return true
		}
	}

	return false
}
