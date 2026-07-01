package handlers_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/require"
)

func TestRSVPModels_Migrate_And_CRUD(t *testing.T) {
	db := testutil.SetupTestDB(t)
	org := testutil.CreateTestOrganization(t, db)

	event := models.RSVPEvent{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		Name:            "Annual Gala",
		Status:          models.RSVPEventStatusDraft,
		Keyword:         "GALA",
		AttendanceField: "attendance",
		AttendanceMap:   models.JSONB{"yes": "yes", "no": "no", "maybe": "maybe"},
	}
	require.NoError(t, db.Create(&event).Error)

	resp := models.RSVPResponse{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		RSVPEventID:    event.ID,
		OrganizationID: org.ID,
		ContactID:      uuid.New(),
		PhoneNumber:    "15551230000",
		Attendance:     models.RSVPAttendancePending,
		Answers:        models.JSONB{},
	}
	require.NoError(t, db.Create(&resp).Error)

	var got models.RSVPResponse
	require.NoError(t, db.First(&got, "id = ?", resp.ID).Error)
	require.Equal(t, models.RSVPAttendancePending, got.Attendance)
}
