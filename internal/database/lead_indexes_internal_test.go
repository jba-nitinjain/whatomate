package database

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A soft-deleted lead source or mapping must not block reconnecting the same
// Page / form, while two live rows must still conflict.
func TestLeadPartialUniqueIndexes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	for _, stmt := range getIndexes() {
		if strings.Contains(stmt, "lead_") {
			require.NoError(t, db.Exec(stmt).Error, stmt)
		}
	}

	org := testutil.CreateTestOrganization(t, db)
	pageID := "page-" + uuid.NewString()[:12]
	newSource := func() *models.LeadSource {
		return &models.LeadSource{
			BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org.ID, Name: "p",
			PageID: pageID, PageAccessToken: "t", WebhookVerifyToken: uuid.NewString(),
		}
	}

	first := newSource()
	require.NoError(t, db.Create(first).Error)
	assert.Error(t, db.Create(newSource()).Error, "two live sources for one page must conflict")

	mapping := &models.LeadFormMapping{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org.ID, LeadSourceID: first.ID, FormID: "f1"}
	require.NoError(t, db.Create(mapping).Error)
	require.NoError(t, db.Delete(mapping).Error)
	require.NoError(t, db.Create(&models.LeadFormMapping{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org.ID, LeadSourceID: first.ID, FormID: "f1"}).Error,
		"re-creating a soft-deleted form mapping must succeed")

	require.NoError(t, db.Delete(first).Error)
	require.NoError(t, db.Create(newSource()).Error, "reconnecting a soft-deleted page must succeed")
}
