package database

import (
	"fmt"

	"github.com/nikyjain/whatomate/internal/models"
	"gorm.io/gorm"
)

// SyncAdminRolePermissions grants every known permission to existing system
// "admin" roles. FixSystemRolePermissions only seeds roles that have no
// permissions at all, so permissions introduced later (e.g. audit_logs, rsvp,
// lead_ads) would otherwise never reach admins of existing organizations and
// lock them out of new screens. This is additive: it never removes a link.
func SyncAdminRolePermissions(db *gorm.DB) error {
	var permissions []models.Permission
	if err := db.Find(&permissions).Error; err != nil {
		return fmt.Errorf("failed to fetch permissions: %w", err)
	}
	if len(permissions) == 0 {
		return nil
	}

	var adminRoles []models.CustomRole
	if err := db.Where("is_system = ? AND name = ?", true, "admin").Find(&adminRoles).Error; err != nil {
		return fmt.Errorf("failed to fetch admin roles: %w", err)
	}

	for _, role := range adminRoles {
		var linked []string
		if err := db.Table("role_permissions").
			Where("custom_role_id = ?", role.ID).
			Pluck("permission_id", &linked).Error; err != nil {
			return fmt.Errorf("failed to read permissions of role %s: %w", role.ID, err)
		}
		have := make(map[string]struct{}, len(linked))
		for _, id := range linked {
			have[id] = struct{}{}
		}

		var missing []models.Permission
		for _, p := range permissions {
			if _, ok := have[p.ID.String()]; !ok {
				missing = append(missing, p)
			}
		}
		if len(missing) == 0 {
			continue
		}
		if err := db.Model(&role).Association("Permissions").Append(missing); err != nil {
			return fmt.Errorf("failed to grant permissions to admin role %s: %w", role.ID, err)
		}
	}
	return nil
}
