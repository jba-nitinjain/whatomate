package handlers

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

func TestRSVPImportParsingAndPhoneNormalization(t *testing.T) {
	rows, err := readRSVPGuestRows("guests.csv", strings.NewReader("name,mobile\nAlice,9876543210\n"))
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, 1, rsvpImportColumn(rows[0], "phone", "mobile"))
	assert.Equal(t, "919876543210", normalizeRSVPImportPhone(rows[1][1]))
	assert.Empty(t, normalizeRSVPImportPhone("123"))

	book := excelize.NewFile()
	require.NoError(t, book.SetSheetRow("Sheet1", "A1", &[]interface{}{"guest_name", "phone_number"}))
	require.NoError(t, book.SetSheetRow("Sheet1", "A2", &[]interface{}{"Bob", "919999999999"}))
	var data bytes.Buffer
	require.NoError(t, book.Write(&data))
	xlsxRows, err := readRSVPGuestRows("guests.xlsx", bytes.NewReader(data.Bytes()))
	require.NoError(t, err)
	require.Len(t, xlsxRows, 2)
	assert.Equal(t, "Bob", xlsxRows[1][0])
}

func TestLoadNotStartedRSVPGuests(t *testing.T) {
	db := testutil.SetupTestDB(t)
	app := &App{DB: db, Log: testutil.NopLogger()}
	org := testutil.CreateTestOrganization(t, db)
	contactA := testutil.CreateTestContact(t, db, org.ID)
	contactB := testutil.CreateTestContact(t, db, org.ID)
	event := models.RSVPEvent{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org.ID, Name: "Roster", Status: models.RSVPEventStatusActive, AccessMode: models.RSVPAccessModeGuestList, CreatedBy: uuid.New()}
	require.NoError(t, db.Create(&event).Error)
	now := time.Now().UTC()
	rows := []models.RSVPResponse{
		{BaseModel: models.BaseModel{ID: uuid.New()}, RSVPEventID: event.ID, OrganizationID: org.ID, ContactID: contactA.ID, PhoneNumber: contactA.PhoneNumber, Attendance: models.RSVPAttendancePending, Source: models.RSVPGuestSourceContactSelection},
		{BaseModel: models.BaseModel{ID: uuid.New()}, RSVPEventID: event.ID, OrganizationID: org.ID, ContactID: contactB.ID, PhoneNumber: contactB.PhoneNumber, Attendance: models.RSVPAttendancePending, Source: models.RSVPGuestSourceContactSelection, RSVPStartedAt: &now},
	}
	require.NoError(t, db.Create(&rows).Error)

	eligible, err := app.loadNotStartedRSVPGuests(org.ID, event.ID, nil, nil)
	require.NoError(t, err)
	require.Len(t, eligible, 1)
	assert.Equal(t, contactA.ID, eligible[0].ContactID)

	excluded, err := app.loadNotStartedRSVPGuests(org.ID, event.ID, nil, []uuid.UUID{rows[0].ID})
	require.NoError(t, err)
	assert.Empty(t, excluded)
	assert.True(t, app.rsvpGuestListed(event.ID, contactA.ID))
	assert.False(t, app.rsvpGuestListed(event.ID, uuid.New()))
}

func TestRSVPAccessModeValidation(t *testing.T) {
	assert.True(t, validRSVPAccessMode(models.RSVPAccessModeGuestList))
	assert.True(t, validRSVPAccessMode(models.RSVPAccessModeOpenKeyword))
	assert.False(t, validRSVPAccessMode(models.RSVPAccessMode("public")))
}

func TestResolveRSVPReminderParams(t *testing.T) {
	eventDate := time.Date(2026, time.July, 19, 0, 0, 0, 0, time.UTC)
	event := &models.RSVPEvent{Name: "Annual Gathering", Description: "Main hall", Keyword: "JOIN", EventDate: &eventDate}
	contact := &models.Contact{ProfileName: "Asha Member", PhoneNumber: "919876543210"}
	response := &models.RSVPResponse{PhoneNumber: contact.PhoneNumber, Contact: contact, Answers: models.JSONB{"city": "Chennai"}}

	got := resolveRSVPReminderParams(map[string]string{
		"1": "{{member_name}}",
		"2": "{{event_name}} on {{event_date}}",
		"3": "Desk A",
		"4": "{{answer.city}}",
	}, event, response)

	assert.Equal(t, "Asha Member", got["1"])
	assert.Equal(t, "Annual Gathering on 19/07/2026", got["2"])
	assert.Equal(t, "Desk A", got["3"])
	assert.Equal(t, "Chennai", got["4"])
}

func TestValidateRSVPReminderParams(t *testing.T) {
	template := &models.Template{BodyContent: "Hello {{1}}, welcome to {{event_label}}"}
	err := validateRSVPReminderParams(template, map[string]string{"1": "{{member_name}}"})
	require.EqualError(t, err, "map reminder template parameters: event_label")
	require.NoError(t, validateRSVPReminderParams(template, map[string]string{
		"1": "{{member_name}}", "event_label": "{{event_name}}",
	}))
}
