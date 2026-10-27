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

// managerCatalogGrantKey marks the one-time grant of catalog permissions to
// existing manager roles. Catalog endpoints were previously open to any
// signed-in user; new orgs give managers these permissions by default, so
// existing managers receive them once. Recording the grant means an admin who
// later removes them from a manager role is not overridden on the next boot.
const managerCatalogGrantKey = "grant_catalogs_to_managers_v1"

// GrantCatalogPermissionsToManagers gives existing system "manager" roles the
// catalogs permissions exactly once. Additive only.
func GrantCatalogPermissionsToManagers(db *gorm.DB) error {
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS permission_grants_applied (
		key TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`).Error; err != nil {
		return fmt.Errorf("failed to create permission_grants_applied: %w", err)
	}

	var applied int64
	if err := db.Table("permission_grants_applied").Where("key = ?", managerCatalogGrantKey).Count(&applied).Error; err != nil {
		return fmt.Errorf("failed to read permission grant marker: %w", err)
	}
	if applied > 0 {
		return nil
	}

	var catalogPerms []models.Permission
	if err := db.Where("resource = ?", models.ResourceCatalogs).Find(&catalogPerms).Error; err != nil {
		return fmt.Errorf("failed to fetch catalog permissions: %w", err)
	}
	if len(catalogPerms) == 0 {
		return nil
	}

	var managerRoles []models.CustomRole
	if err := db.Where("is_system = ? AND name = ?", true, "manager").Find(&managerRoles).Error; err != nil {
		return fmt.Errorf("failed to fetch manager roles: %w", err)
	}

	for _, role := range managerRoles {
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
		for _, p := range catalogPerms {
			if _, ok := have[p.ID.String()]; !ok {
				missing = append(missing, p)
			}
		}
		if len(missing) == 0 {
			continue
		}
		if err := db.Model(&role).Association("Permissions").Append(missing); err != nil {
			return fmt.Errorf("failed to grant catalog permissions to manager role %s: %w", role.ID, err)
		}
	}

	if err := db.Exec("INSERT INTO permission_grants_applied (key) VALUES (?) ON CONFLICT DO NOTHING", managerCatalogGrantKey).Error; err != nil {
		return fmt.Errorf("failed to record permission grant marker: %w", err)
	}
	return nil
}
