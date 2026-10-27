package database

import (
	"testing"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Existing manager roles get the catalogs permissions once; an admin who later
// removes them must not see them come back on the next boot.
func TestGrantCatalogPermissionsToManagers_OneTime(t *testing.T) {
	db := testutil.SetupIsolatedTestDB(t)
	require.NoError(t, SeedPermissionsAndRoles(db))

	org := testutil.CreateTestOrganization(t, db)
	manager := &models.CustomRole{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org.ID, Name: "manager", IsSystem: true}
	require.NoError(t, db.Create(manager).Error)
	custom := &models.CustomRole{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org.ID, Name: "support", IsSystem: false}
	require.NoError(t, db.Create(custom).Error)

	catalogCount := func(roleID uuid.UUID) int64 {
		var n int64
		require.NoError(t, db.Table("role_permissions").
			Joins("JOIN permissions ON permissions.id = role_permissions.permission_id").
			Where("role_permissions.custom_role_id = ? AND permissions.resource = ?", roleID, models.ResourceCatalogs).
			Count(&n).Error)
		return n
	}

	require.NoError(t, GrantCatalogPermissionsToManagers(db))
	assert.EqualValues(t, 4, catalogCount(manager.ID), "existing manager gets all catalog permissions")
	assert.EqualValues(t, 0, catalogCount(custom.ID), "custom roles are untouched")

	require.NoError(t, db.Exec(`DELETE FROM role_permissions WHERE custom_role_id = ? AND permission_id IN
		(SELECT id FROM permissions WHERE resource = ?)`, manager.ID, models.ResourceCatalogs).Error)
	require.NoError(t, GrantCatalogPermissionsToManagers(db))
	assert.EqualValues(t, 0, catalogCount(manager.ID), "grant runs only once")
}
