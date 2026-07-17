package handlers

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/config"
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

func TestCreateRSVPReminderCampaignQueuesResolvedRecipients(t *testing.T) {
	db := testutil.SetupTestDB(t)
	mockQueue := testutil.NewMockQueue()
	app := &App{DB: db, Log: testutil.NopLogger(), Queue: mockQueue}
	org := testutil.CreateTestOrganization(t, db)
	user := testutil.CreateTestUser(t, db, org.ID)
	account := testutil.CreateTestWhatsAppAccount(t, db, org.ID)
	template := testutil.CreateTestTemplate(t, db, org.ID, account.Name)
	contact := testutil.CreateTestContactWith(t, db, org.ID,
		testutil.WithContactAccount(account.Name),
		testutil.WithPhoneNumber("919876543210"),
	)
	event := models.RSVPEvent{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		Name:            "Annual Gathering",
		Status:          models.RSVPEventStatusActive,
		AccessMode:      models.RSVPAccessModeGuestList,
		WhatsAppAccount: account.Name,
		CreatedBy:       user.ID,
	}
	require.NoError(t, db.Create(&event).Error)
	response := models.RSVPResponse{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		RSVPEventID:    event.ID,
		OrganizationID: org.ID,
		ContactID:      contact.ID,
		PhoneNumber:    contact.PhoneNumber,
		Attendance:     models.RSVPAttendancePending,
		Source:         models.RSVPGuestSourceContactSelection,
	}
	require.NoError(t, db.Create(&response).Error)

	result, err := app.createRSVPReminderCampaign(
		context.Background(),
		&event,
		template,
		map[string]string{"1": "{{member_name}} for {{event_name}}"},
		[]models.RSVPResponse{response},
		models.RSVPReminderDeliveryManual,
		nil,
		user.ID,
		"", "", "",
	)
	require.NoError(t, err)
	require.NotNil(t, result.Campaign)
	assert.Equal(t, 1, result.Queued)
	assert.Zero(t, len(result.Skipped))
	assert.Equal(t, models.CampaignSourceRSVPReminder, result.Campaign.SourceType)
	require.NotNil(t, result.Campaign.SourceID)
	assert.Equal(t, event.ID, *result.Campaign.SourceID)
	assert.Equal(t, models.CampaignStatusProcessing, result.Campaign.Status)
	assert.Equal(t, 1, mockQueue.JobCount())

	var recipient models.BulkMessageRecipient
	require.NoError(t, db.Where("campaign_id = ?", result.Campaign.ID).First(&recipient).Error)
	assert.Equal(t, contact.PhoneNumber, recipient.PhoneNumber)
	assert.Equal(t, contact.ProfileName+" for Annual Gathering", recipient.TemplateParams["1"])

	var delivery models.RSVPReminderDelivery
	require.NoError(t, db.Where("campaign_recipient_id = ?", recipient.ID).First(&delivery).Error)
	assert.Equal(t, models.RSVPReminderDeliveryQueued, delivery.Status)
	assert.Equal(t, response.ID, delivery.RSVPResponseID)
	require.NotNil(t, delivery.CampaignID)
	assert.Equal(t, result.Campaign.ID, *delivery.CampaignID)
	require.NotNil(t, delivery.InitiatedBy)
	assert.Equal(t, user.ID, *delivery.InitiatedBy)
}

// TestCreateRSVPReminderCampaignPromotesStagedMediaToPublicURL proves the fix
// for the 15/07/2026 failure end-to-end at the unit level: a VIDEO-header
// template with a staged attachment must come out with HeaderMediaURL set to
// a fetchable link, because worker.go:122 only ever sends HeaderMediaID or
// HeaderMediaURL - HeaderMediaLocalPath alone (the old behavior) sends no
// header component at all and Meta rejects the message with error 132012.
func TestCreateRSVPReminderCampaignPromotesStagedMediaToPublicURL(t *testing.T) {
	db := testutil.SetupTestDB(t)
	mockQueue := testutil.NewMockQueue()
	app := &App{
		DB:    db,
		Log:   testutil.NopLogger(),
		Queue: mockQueue,
		Config: &config.Config{
			Storage: config.StorageConfig{LocalPath: t.TempDir()},
			JWT:     config.JWTConfig{Secret: "test-secret"},
		},
	}
	org := testutil.CreateTestOrganization(t, db)
	user := testutil.CreateTestUser(t, db, org.ID)
	account := testutil.CreateTestWhatsAppAccount(t, db, org.ID)
	template := testutil.CreateTestTemplate(t, db, org.ID, account.Name)
	template.HeaderType = "VIDEO"
	require.NoError(t, db.Save(template).Error)
	contact := testutil.CreateTestContactWith(t, db, org.ID,
		testutil.WithContactAccount(account.Name),
		testutil.WithPhoneNumber("919876543211"),
	)
	event := models.RSVPEvent{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		Name:            "Video Invite",
		Status:          models.RSVPEventStatusActive,
		AccessMode:      models.RSVPAccessModeGuestList,
		WhatsAppAccount: account.Name,
		CreatedBy:       user.ID,
	}
	require.NoError(t, db.Create(&event).Error)
	response := models.RSVPResponse{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		RSVPEventID:    event.ID,
		OrganizationID: org.ID,
		ContactID:      contact.ID,
		PhoneNumber:    contact.PhoneNumber,
		Attendance:     models.RSVPAttendancePending,
		Source:         models.RSVPGuestSourceContactSelection,
	}
	require.NoError(t, db.Create(&response).Error)

	// Mirrors what UploadRSVPReminderMedia does when the guest picks a file
	// before sending: stage it under a pseudo campaign id.
	stagingID := uuid.New().String()
	key := rsvpReminderStagingKey(stagingID)
	require.NotEmpty(t, key)
	_, err := app.saveCampaignMedia(key, []byte("fake mp4 bytes"), "video/mp4")
	require.NoError(t, err)

	result, err := app.createRSVPReminderCampaign(
		context.Background(),
		&event,
		template,
		nil,
		[]models.RSVPResponse{response},
		models.RSVPReminderDeliveryManual,
		nil,
		user.ID,
		stagingID, "invite-clip.mp4", "https://reminders.example.test",
	)
	require.NoError(t, err)
	require.NotNil(t, result.Campaign)

	// Reaching CampaignStatusProcessing means validateCampaignReadyForStart
	// accepted the campaign - which after this fix only happens via a real
	// HeaderMediaID/HeaderMediaURL, never HeaderMediaLocalPath alone.
	assert.Equal(t, models.CampaignStatusProcessing, result.Campaign.Status)
	assert.Contains(t, result.Campaign.HeaderMediaURL, "https://reminders.example.test/public/campaigns/"+result.Campaign.ID.String()+"/media/invite-clip.mp4?token=")
	assert.Equal(t, "video/mp4", result.Campaign.HeaderMediaMimeType)
	assert.NotEmpty(t, result.Campaign.HeaderMediaLocalPath)
	assert.Empty(t, result.Campaign.HeaderMediaID)

	var stored models.BulkMessageCampaign
	require.NoError(t, db.First(&stored, "id = ?", result.Campaign.ID).Error)
	assert.Equal(t, result.Campaign.HeaderMediaURL, stored.HeaderMediaURL)
	assert.Equal(t, result.Campaign.HeaderMediaLocalPath, stored.HeaderMediaLocalPath)
	assert.Equal(t, 1, mockQueue.JobCount())
}

// TestCreateRSVPReminderCampaignFailsCleanlyWhenMediaMissing pins the fix for
// the post-commit-orphan bug found in review: a VIDEO-header template with no
// staged attachment - exactly what the scheduler hits on every tick, since
// rsvp_scheduler.go always passes stagingID "" - used to commit the campaign,
// recipient and delivery rows in a DB transaction and only THEN run
// validateCampaignReadyForStart, so a failure here left those rows behind
// forever: recipients stuck "pending", deliveries stuck "queued", nothing to
// retry them. That is how 1008 reminders failed on 15/07/2026 while still
// writing full campaign state.
//
// This test would fail under the old post-commit-validation ordering: it
// would find a persisted campaign row (and recipient/delivery rows) even
// though err is non-nil. Requires TEST_DATABASE_URL; skips otherwise per
// testutil.SetupTestDB.
func TestCreateRSVPReminderCampaignFailsCleanlyWhenMediaMissing(t *testing.T) {
	db := testutil.SetupTestDB(t)
	mockQueue := testutil.NewMockQueue()
	app := &App{
		DB:    db,
		Log:   testutil.NopLogger(),
		Queue: mockQueue,
		Config: &config.Config{
			Storage: config.StorageConfig{LocalPath: t.TempDir()},
			JWT:     config.JWTConfig{Secret: "test-secret"},
		},
	}
	org := testutil.CreateTestOrganization(t, db)
	user := testutil.CreateTestUser(t, db, org.ID)
	account := testutil.CreateTestWhatsAppAccount(t, db, org.ID)
	template := testutil.CreateTestTemplate(t, db, org.ID, account.Name)
	template.HeaderType = "VIDEO"
	require.NoError(t, db.Save(template).Error)
	contact := testutil.CreateTestContactWith(t, db, org.ID,
		testutil.WithContactAccount(account.Name),
		testutil.WithPhoneNumber("919876543212"),
	)
	event := models.RSVPEvent{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		Name:            "Video Invite No Media",
		Status:          models.RSVPEventStatusActive,
		AccessMode:      models.RSVPAccessModeGuestList,
		WhatsAppAccount: account.Name,
		CreatedBy:       user.ID,
	}
	require.NoError(t, db.Create(&event).Error)
	response := models.RSVPResponse{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		RSVPEventID:    event.ID,
		OrganizationID: org.ID,
		ContactID:      contact.ID,
		PhoneNumber:    contact.PhoneNumber,
		Attendance:     models.RSVPAttendancePending,
		Source:         models.RSVPGuestSourceContactSelection,
	}
	require.NoError(t, db.Create(&response).Error)

	// stagingID "" mirrors both a manual send where the admin never attached
	// media and every scheduled run (rsvp_scheduler.go always passes "").
	result, err := app.createRSVPReminderCampaign(
		context.Background(),
		&event,
		template,
		nil,
		[]models.RSVPResponse{response},
		models.RSVPReminderDeliveryScheduled,
		nil,
		user.ID,
		"", "", "",
	)
	require.Error(t, err)
	assert.Nil(t, result.Campaign)
	assert.Zero(t, result.Queued)
	assert.Equal(t, 0, mockQueue.JobCount())

	var campaignCount int64
	require.NoError(t, db.Model(&models.BulkMessageCampaign{}).
		Where("source_type = ? AND source_id = ?", models.CampaignSourceRSVPReminder, event.ID).
		Count(&campaignCount).Error)
	assert.Zero(t, campaignCount, "no campaign row should exist for a send that cannot attach required media")

	var recipientCount int64
	require.NoError(t, db.Model(&models.BulkMessageRecipient{}).
		Where("phone_number = ?", contact.PhoneNumber).
		Count(&recipientCount).Error)
	assert.Zero(t, recipientCount, "no recipient row should exist for a send that cannot attach required media")

	var deliveryCount int64
	require.NoError(t, db.Model(&models.RSVPReminderDelivery{}).
		Where("rsvp_response_id = ?", response.ID).
		Count(&deliveryCount).Error)
	assert.Zero(t, deliveryCount, "no delivery row should exist for a send that cannot attach required media")
}
