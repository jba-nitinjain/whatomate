package handlers

import (
	"time"

	"github.com/shridarpatil/whatomate/internal/models"
)

type chatRepairSampleMessageRow struct {
	ContactID     string    `gorm:"column:contact_id"`
	Direction     string    `gorm:"column:direction"`
	MessageType   string    `gorm:"column:message_type"`
	Content       string    `gorm:"column:content"`
	MediaFilename string    `gorm:"column:media_filename"`
	TemplateName  string    `gorm:"column:template_name"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

func (a *App) loadChatRepairSampleMessages(contactIDs []string, limitPerContact int) (map[string][]ChatRepairSampleMessage, error) {
	if len(contactIDs) == 0 || limitPerContact <= 0 {
		return map[string][]ChatRepairSampleMessage{}, nil
	}

	query := `
		SELECT
			x.contact_id,
			x.direction,
			x.message_type,
			x.content,
			x.media_filename,
			x.template_name,
			x.created_at
		FROM (
			SELECT
				m.contact_id::text AS contact_id,
				m.direction,
				m.message_type,
				m.content,
				m.media_filename,
				m.template_name,
				m.created_at,
				ROW_NUMBER() OVER (PARTITION BY m.contact_id ORDER BY m.created_at DESC) AS row_num
			FROM messages m
			WHERE m.deleted_at IS NULL
				AND m.contact_id::text IN ?
		) x
		WHERE x.row_num <= ?
		ORDER BY x.contact_id, x.created_at DESC
	`

	var rows []chatRepairSampleMessageRow
	if err := a.DB.Raw(query, contactIDs, limitPerContact).Scan(&rows).Error; err != nil {
		return nil, err
	}

	samples := make(map[string][]ChatRepairSampleMessage, len(contactIDs))
	for _, row := range rows {
		msg := &models.Message{
			Direction:       models.Direction(row.Direction),
			MessageType:     models.MessageType(row.MessageType),
			Content:         row.Content,
			MediaFilename:   row.MediaFilename,
			TemplateName:    row.TemplateName,
		}

		samples[row.ContactID] = append(samples[row.ContactID], ChatRepairSampleMessage{
			Direction:   row.Direction,
			MessageType: row.MessageType,
			Preview:     a.getPersistedMessagePreview(msg),
			CreatedAt:   row.CreatedAt,
		})
	}

	return samples, nil
}
