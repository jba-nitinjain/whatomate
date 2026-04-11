package worker

import (
	"fmt"
	"time"

	"github.com/nikyjain/whatomate/internal/models"
	"github.com/zerodha/logf"
	"gorm.io/gorm"
)

// StartMessageRetentionWorker runs the message retention cleanup once at startup
// and then once every 24 hours. It deletes messages older than the number of days
// configured in each organisation's message_retention_days setting (1-60 days).
// When the setting is 0 or absent, no messages are deleted for that organisation.
func StartMessageRetentionWorker(db *gorm.DB, log logf.Logger) {
	// Run immediately at startup, then on a daily ticker.
	runMessageRetentionCleanup(db, log)

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		runMessageRetentionCleanup(db, log)
	}
}

// runMessageRetentionCleanup performs one pass of the retention cleanup across all
// organisations that have message_retention_days configured.
func runMessageRetentionCleanup(db *gorm.DB, log logf.Logger) {
	log.Info("message retention: starting cleanup pass")

	// Find all organisations with a positive retention setting.
	type orgRetention struct {
		ID                   string
		MessageRetentionDays int
	}

	var rows []orgRetention
	if err := db.Raw(`
		SELECT id::text, (settings->>'message_retention_days')::int AS message_retention_days
		FROM organizations
		WHERE deleted_at IS NULL
		  AND settings->>'message_retention_days' IS NOT NULL
		  AND (settings->>'message_retention_days')::int > 0
		  AND (settings->>'message_retention_days')::int <= 60
	`).Scan(&rows).Error; err != nil {
		log.Error("message retention: failed to query organisations", "error", err)
		return
	}

	if len(rows) == 0 {
		log.Info("message retention: no organisations with active retention policy")
		return
	}

	total := int64(0)
	for _, r := range rows {
		cutoff := time.Now().AddDate(0, 0, -r.MessageRetentionDays)

		result := db.Unscoped().
			Where("organization_id = ? AND created_at < ? AND deleted_at IS NULL", r.ID, cutoff).
			Delete(&models.Message{})

		if result.Error != nil {
			log.Error("message retention: failed to delete messages",
				"org_id", r.ID,
				"error", fmt.Sprintf("%v", result.Error),
			)
			continue
		}

		if result.RowsAffected > 0 {
			log.Info("message retention: deleted old messages",
				"org_id", r.ID,
				"retention_days", r.MessageRetentionDays,
				"deleted_count", result.RowsAffected,
			)
			total += result.RowsAffected
		}
	}

	log.Info("message retention: cleanup pass complete", "total_deleted", total)
}
