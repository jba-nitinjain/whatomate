package handlers_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/require"
)

func TestRSVPEventPersistsHeadcountContributors(t *testing.T) {
	db := testutil.SetupTestDB(t)
	org := testutil.CreateTestOrganization(t, db)

	event := models.RSVPEvent{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		Name:            "Golden Jubilee",
		Status:          models.RSVPEventStatusDraft,
		Keyword:         "JUBILEE",
		AttendanceField: "attendance",
		HeadcountContributors: models.RSVPHeadcountContributors{
			{Label: "Children", AnswerKey: "children_count", Mode: models.RSVPHeadcountModeNumeric},
		},
	}
	require.NoError(t, db.Create(&event).Error)

	var got models.RSVPEvent
	require.NoError(t, db.First(&got, "id = ?", event.ID).Error)
	require.Len(t, got.HeadcountContributors, 1)
	require.Equal(t, "children_count", got.HeadcountContributors[0].AnswerKey)
	require.Equal(t, models.RSVPHeadcountModeNumeric, got.HeadcountContributors[0].Mode)
}

func TestRSVPEventWithoutContributorsScansEmpty(t *testing.T) {
	// Mirrors the live event, created before this column existed.
	db := testutil.SetupTestDB(t)
	org := testutil.CreateTestOrganization(t, db)

	event := models.RSVPEvent{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		Name:            "Legacy",
		Status:          models.RSVPEventStatusActive,
		Keyword:         "LEGACY",
		AttendanceField: "attendance",
	}
	require.NoError(t, db.Create(&event).Error)

	var got models.RSVPEvent
	require.NoError(t, db.First(&got, "id = ?", event.ID).Error)
	require.Empty(t, got.HeadcountContributors)
}
