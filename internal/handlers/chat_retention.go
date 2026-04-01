package handlers

import (
	"context"
	"time"

	"github.com/shridarpatil/whatomate/internal/models"
)

const (
	defaultChatRetentionPeriod   = 60 * 24 * time.Hour
	defaultChatRetentionInterval = 24 * time.Hour
)

// StartChatRetentionProcessor starts a background loop that permanently removes
// old chat messages from the database and repairs affected contact summaries.
func (a *App) StartChatRetentionProcessor(retentionPeriod, interval time.Duration) {
	if retentionPeriod <= 0 {
		retentionPeriod = defaultChatRetentionPeriod
	}
	if interval <= 0 {
		interval = defaultChatRetentionInterval
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.ChatRetentionCancel = cancel

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		a.ProcessExpiredChatData(ctx, retentionPeriod)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.ProcessExpiredChatData(ctx, retentionPeriod)
			}
		}
	}()
}

// StopChatRetentionProcessor stops the background chat retention loop.
func (a *App) StopChatRetentionProcessor() {
	if a.ChatRetentionCancel != nil {
		a.ChatRetentionCancel()
	}
}

// ProcessExpiredChatData permanently deletes message rows older than the
// retention window and recalculates contact chat summaries for impacted chats.
func (a *App) ProcessExpiredChatData(ctx context.Context, retentionPeriod time.Duration) {
	cutoff := time.Now().Add(-retentionPeriod)

	tx := a.DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		a.Log.Error("Failed to start chat retention transaction", "error", tx.Error)
		return
	}

	var staleCount int64
	if err := tx.Model(&models.Message{}).
		Where("deleted_at IS NULL AND created_at < ?", cutoff).
		Count(&staleCount).Error; err != nil {
		tx.Rollback()
		a.Log.Error("Failed to count stale chat messages", "error", err, "cutoff", cutoff)
		return
	}

	if staleCount == 0 {
		tx.Rollback()
		return
	}

	staleMessages := tx.Model(&models.Message{}).
		Select("id").
		Where("deleted_at IS NULL AND created_at < ?", cutoff)

	if err := tx.Model(&models.Message{}).
		Where("reply_to_message_id IN (?)", staleMessages).
		Update("reply_to_message_id", nil).Error; err != nil {
		tx.Rollback()
		a.Log.Error("Failed to detach replies before chat retention delete", "error", err, "cutoff", cutoff)
		return
	}

	if err := tx.Exec(`
WITH affected_contacts AS (
    SELECT DISTINCT contact_id
    FROM messages
    WHERE deleted_at IS NULL AND created_at < ?
),
latest_message AS (
    SELECT DISTINCT ON (m.contact_id)
        m.contact_id,
        m.created_at,
        CASE
            WHEN m.message_type = 'text' THEN LEFT(COALESCE(m.content, ''), 100)
            WHEN m.message_type = 'image' THEN CASE
                WHEN COALESCE(m.content, '') <> '' THEN LEFT(m.content, 100)
                ELSE '[Image]'
            END
            WHEN m.message_type = 'video' THEN CASE
                WHEN COALESCE(m.content, '') <> '' THEN LEFT(m.content, 100)
                ELSE '[Video]'
            END
            WHEN m.message_type = 'audio' THEN '[Audio]'
            WHEN m.message_type = 'document' THEN CASE
                WHEN COALESCE(m.media_filename, '') <> '' THEN '[Document: ' || m.media_filename || ']'
                ELSE '[Document]'
            END
            WHEN m.message_type = 'interactive' THEN LEFT(COALESCE(m.content, ''), 100)
            WHEN m.message_type = 'template' THEN CASE
                WHEN COALESCE(m.template_name, '') <> '' THEN '[Template: ' || m.template_name || ']'
                ELSE '[Template]'
            END
            ELSE '[Message]'
        END AS preview
    FROM messages m
    JOIN affected_contacts ac ON ac.contact_id = m.contact_id
    WHERE m.deleted_at IS NULL AND m.created_at >= ?
    ORDER BY m.contact_id, m.created_at DESC, m.id DESC
),
contact_unread AS (
    SELECT
        m.contact_id,
        BOOL_OR(m.direction = 'incoming' AND m.status <> 'read') AS has_unread,
        MAX(CASE WHEN m.direction = 'incoming' THEN m.created_at END) AS last_inbound_at
    FROM messages m
    JOIN affected_contacts ac ON ac.contact_id = m.contact_id
    WHERE m.deleted_at IS NULL AND m.created_at >= ?
    GROUP BY m.contact_id
)
UPDATE contacts c
SET
    last_message_at = lm.created_at,
    last_message_preview = COALESCE(lm.preview, ''),
    is_read = COALESCE(NOT cu.has_unread, TRUE),
    last_inbound_at = cu.last_inbound_at
FROM affected_contacts ac
LEFT JOIN latest_message lm ON lm.contact_id = ac.contact_id
LEFT JOIN contact_unread cu ON cu.contact_id = ac.contact_id
WHERE c.id = ac.contact_id
`, cutoff, cutoff, cutoff).Error; err != nil {
		tx.Rollback()
		a.Log.Error("Failed to recalculate contacts during chat retention", "error", err, "cutoff", cutoff)
		return
	}

	result := tx.Unscoped().
		Where("created_at < ?", cutoff).
		Delete(&models.Message{})
	if result.Error != nil {
		tx.Rollback()
		a.Log.Error("Failed to delete stale chat messages", "error", result.Error, "cutoff", cutoff)
		return
	}

	if err := tx.Commit().Error; err != nil {
		a.Log.Error("Failed to commit chat retention transaction", "error", err, "cutoff", cutoff)
		return
	}

	a.Log.Info("Expired chat messages deleted",
		"cutoff", cutoff,
		"retention_hours", retentionPeriod.Hours(),
		"deleted_messages", result.RowsAffected,
	)
}
