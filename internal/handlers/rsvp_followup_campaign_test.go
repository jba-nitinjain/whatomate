package handlers

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/config"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

// TestRSVPFollowUpCampaignQueuesResolvedRecipients is the follow-up
// happy path mirroring TestCreateRSVPReminderCampaignQueuesResolvedRecipients
// (rsvp_guest_reminder_test.go): a plain-text template with one eligible
// guest must produce a processing campaign, one queued job, and - the part
// specific to follow-ups - a FlowID recorded on the campaign so the chatbot
// hook (Task 6) knows which flow to run when the guest taps through.
//
// Named with the TestRSVPFollowUp prefix (rather than TestCreateRSVPFollowUp)
// so it - and the media-missing regression test below - are caught by the
// task's verification filter `-run 'TestRSVPFollowUp|...'`.
func TestRSVPFollowUpCampaignQueuesResolvedRecipients(t *testing.T) {
	db := testutil.SetupTestDB(t)
	mockQueue := testutil.NewMockQueue()
	app := &App{DB: db, Log: testutil.NopLogger(), Queue: mockQueue}
	org := testutil.CreateTestOrganization(t, db)
	user := testutil.CreateTestUser(t, db, org.ID)
	account := testutil.CreateTestWhatsAppAccount(t, db, org.ID)
	template := testutil.CreateTestTemplate(t, db, org.ID, account.Name)
	contact := testutil.CreateTestContactWith(t, db, org.ID,
		testutil.WithContactAccount(account.Name),
		testutil.WithPhoneNumber("919876543220"),
	)
	flow := models.ChatbotFlow{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		WhatsAppAccount: account.Name,
		Name:            "Follow-up flow",
	}
	require.NoError(t, db.Create(&flow).Error)
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
		Attendance:     models.RSVPAttendanceYes,
		Answers:        models.JSONB{"attendance": "yes"},
		Source:         models.RSVPGuestSourceContactSelection,
	}
	require.NoError(t, db.Create(&response).Error)
	rows := []rsvpGuestRosterRow{{RSVPResponse: response, JourneyStatus: "responded"}}
	// Contact is not preloaded on the row above (unlike loadRSVPFollowUpGuests,
	// which always preloads it) - set it directly so eligibility does not skip
	// this guest for "no contact record", the same shortcut
	// TestRSVPFollowUpEligibilitySkipsMissingContactAndPhone's rows take.
	rows[0].Contact = contact

	result, err := app.createRSVPFollowUpCampaign(
		context.Background(),
		&event,
		template,
		flow.ID,
		map[string]string{"1": "{{member_name}} for {{event_name}}"},
		rows,
		user.ID,
		"", "", "",
	)
	require.NoError(t, err)
	require.NotNil(t, result.Campaign)
	assert.Equal(t, 1, result.Queued)
	assert.Zero(t, len(result.Skipped))
	assert.Equal(t, models.CampaignSourceRSVPFollowUp, result.Campaign.SourceType)
	require.NotNil(t, result.Campaign.SourceID)
	assert.Equal(t, event.ID, *result.Campaign.SourceID)
	require.NotNil(t, result.Campaign.FlowID, "campaign must record which flow a tap-through should run")
	assert.Equal(t, flow.ID, *result.Campaign.FlowID)
	assert.Equal(t, models.CampaignStatusProcessing, result.Campaign.Status)
	assert.Equal(t, 1, mockQueue.JobCount())

	var recipient models.BulkMessageRecipient
	require.NoError(t, db.Where("campaign_id = ?", result.Campaign.ID).First(&recipient).Error)
	assert.Equal(t, contact.PhoneNumber, recipient.PhoneNumber)
	assert.Equal(t, contact.ProfileName+" for Annual Gathering", recipient.TemplateParams["1"])
}

// TestRSVPFollowUpCampaignFailsCleanlyWhenMediaMissing is the
// regression test the task requires: mirrors
// TestCreateRSVPReminderCampaignFailsCleanlyWhenMediaMissing
// (rsvp_guest_reminder_test.go) for the follow-up send path. A VIDEO-header
// template with no staged attachment must fail before anything is persisted
// - no campaign row, no recipient row - rather than reproducing the
// 15/07/2026 incident where 1008 reminders committed full campaign state and
// only then failed against Meta with error 132012, while still reporting
// "completed". Requires TEST_DATABASE_URL; skips otherwise per
// testutil.SetupTestDB.
func TestRSVPFollowUpCampaignFailsCleanlyWhenMediaMissing(t *testing.T) {
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
		testutil.WithPhoneNumber("919876543221"),
	)
	flow := models.ChatbotFlow{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		WhatsAppAccount: account.Name,
		Name:            "Follow-up flow",
	}
	require.NoError(t, db.Create(&flow).Error)
	event := models.RSVPEvent{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		Name:            "Video Follow-up No Media",
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
		Attendance:     models.RSVPAttendanceYes,
		Answers:        models.JSONB{"attendance": "yes"},
		Source:         models.RSVPGuestSourceContactSelection,
	}
	require.NoError(t, db.Create(&response).Error)
	rows := []rsvpGuestRosterRow{{RSVPResponse: response, JourneyStatus: "responded"}}
	rows[0].Contact = contact

	// stagingID "" mirrors an admin sending a follow-up without attaching the
	// required header media.
	result, err := app.createRSVPFollowUpCampaign(
		context.Background(),
		&event,
		template,
		flow.ID,
		nil,
		rows,
		user.ID,
		"", "", "",
	)
	require.Error(t, err)
	assert.Nil(t, result.Campaign)
	assert.Zero(t, result.Queued)
	assert.Equal(t, 0, mockQueue.JobCount())

	var campaignCount int64
	require.NoError(t, db.Model(&models.BulkMessageCampaign{}).
		Where("source_type = ? AND source_id = ?", models.CampaignSourceRSVPFollowUp, event.ID).
		Count(&campaignCount).Error)
	assert.Zero(t, campaignCount, "no campaign row should exist for a follow-up send that cannot attach required media")

	var recipientCount int64
	require.NoError(t, db.Model(&models.BulkMessageRecipient{}).
		Where("phone_number = ?", contact.PhoneNumber).
		Count(&recipientCount).Error)
	assert.Zero(t, recipientCount, "no recipient row should exist for a follow-up send that cannot attach required media")
}

// TestRSVPFollowUpCampaignErrorEnvelope mirrors
// TestRSVPReminderCampaignErrorEnvelope (rsvp_reminder_campaign_test.go): a
// rsvpUserFacingError from createRSVPFollowUpCampaign must surface its own
// message as a 400 (even wrapped further by %w), while a plain
// infrastructure error stays a generic 500 rather than leaking internals.
func TestRSVPFollowUpCampaignErrorEnvelope(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "user-facing error surfaces its own message as a 400",
			err:        rsvpUserFacingError{fmt.Errorf("template requires video header media. Configure campaign media before starting")},
			wantStatus: fasthttp.StatusBadRequest,
			wantMsg:    "template requires video header media. Configure campaign media before starting",
		},
		{
			name:       "user-facing error wrapped further by %w is still detected",
			err:        fmt.Errorf("promote staged media: %w", rsvpUserFacingError{fmt.Errorf("failed to read staged media: file removed")}),
			wantStatus: fasthttp.StatusBadRequest,
			wantMsg:    "failed to read staged media: file removed",
		},
		{
			name:       "plain infrastructure error stays a generic 500, not leaked",
			err:        fmt.Errorf("campaign queue is unavailable"),
			wantStatus: fasthttp.StatusInternalServerError,
			wantMsg:    "Failed to create follow-up campaign",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, msg := rsvpFollowUpCampaignErrorEnvelope(c.err)
			if status != c.wantStatus || msg != c.wantMsg {
				t.Fatalf("rsvpFollowUpCampaignErrorEnvelope(%v) = (%d, %q), want (%d, %q)", c.err, status, msg, c.wantStatus, c.wantMsg)
			}
		})
	}
}

// TestRSVPFollowUpFlowIsEventPrimary pins the rule that a follow-up may never
// run the event's own primary RSVP flow (it would re-ask attendance and, via
// the RSVP merge, produce a confusing half-update).
func TestRSVPFollowUpFlowIsEventPrimary(t *testing.T) {
	primaryFlowID := uuid.New()
	otherFlowID := uuid.New()
	event := &models.RSVPEvent{FlowID: &primaryFlowID}

	assert.True(t, rsvpFollowUpFlowIsEventPrimary(event, primaryFlowID))
	assert.False(t, rsvpFollowUpFlowIsEventPrimary(event, otherFlowID))
	assert.False(t, rsvpFollowUpFlowIsEventPrimary(&models.RSVPEvent{FlowID: nil}, otherFlowID))
}

// TestRSVPFollowUpFlowWrongAccount pins the rule carried over from Task 5's
// review: chatbot_processor.go treats ChatbotFlow.WhatsAppAccount as a hard
// gate against the account a message arrives on, so a follow-up flow scoped
// to a different account from the event would send fine and then silently
// fail to start the moment the guest taps through. That must be rejected up
// front at send time instead.
func TestRSVPFollowUpFlowWrongAccount(t *testing.T) {
	event := &models.RSVPEvent{WhatsAppAccount: "primary-account"}

	assert.True(t, rsvpFollowUpFlowWrongAccount(event, &models.ChatbotFlow{WhatsAppAccount: "other-account"}),
		"a flow scoped to a different account than the event must be rejected")
	assert.False(t, rsvpFollowUpFlowWrongAccount(event, &models.ChatbotFlow{WhatsAppAccount: "primary-account"}),
		"a flow scoped to the event's own account must be allowed")
	assert.False(t, rsvpFollowUpFlowWrongAccount(event, &models.ChatbotFlow{WhatsAppAccount: ""}),
		"a flow with no account restriction (org-level default) must be allowed on any account")
}

// TestRSVPFollowUpRowsFilterByResponseID pins the optional response_ids
// refinement: when supplied and valid, it narrows the audience-loaded roster
// down to exactly those ids; when the caller supplies nothing at all, the
// roster is untouched (the intended default - "use the whole audience").
func TestRSVPFollowUpRowsFilterByResponseID(t *testing.T) {
	keep := uuid.New()
	drop := uuid.New()
	rows := []rsvpGuestRosterRow{
		{RSVPResponse: models.RSVPResponse{BaseModel: models.BaseModel{ID: keep}}},
		{RSVPResponse: models.RSVPResponse{BaseModel: models.BaseModel{ID: drop}}},
	}

	filtered, err := filterRSVPFollowUpRowsByResponseID(rows, []string{keep.String()})
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, keep, filtered[0].ID)

	noneSupplied, err := filterRSVPFollowUpRowsByResponseID(rows, nil)
	require.NoError(t, err)
	assert.Equal(t, rows, noneSupplied)
}

// TestRSVPFollowUpRowsFilterByResponseIDRejectsUnparsableSelection pins the
// fix for the fail-open this task closed: previously, a response_ids slice
// that yielded zero valid ids (e.g. every entry was garbled/stale) fell back
// to returning the full roster untouched - silently sending to the entire
// audience instead of the empty/garbled selection the caller actually sent.
// That is the opposite of what an admin who picked specific guests asked
// for, so it must now return a user-facing error instead.
func TestRSVPFollowUpRowsFilterByResponseIDRejectsUnparsableSelection(t *testing.T) {
	rows := []rsvpGuestRosterRow{
		{RSVPResponse: models.RSVPResponse{BaseModel: models.BaseModel{ID: uuid.New()}}},
	}

	filtered, err := filterRSVPFollowUpRowsByResponseID(rows, []string{"not-a-uuid"})
	require.Error(t, err, "an unparsable response_ids selection must not silently fall back to the whole audience")
	assert.Nil(t, filtered)
	var userErr rsvpUserFacingError
	require.ErrorAs(t, err, &userErr, "must be a rsvpUserFacingError so SendRSVPFollowUp surfaces it as a 400 instead of a generic 500")
}

// TestRSVPFollowUpRowsFilterByResponseIDRejectsNoMatchingRows pins the other
// half of the same fix: response_ids that parse fine but match none of the
// rows in the loaded audience (a stale selection from a previous
// audience/answer_key) must also error rather than silently sending to
// everyone - an admin who selected specific guests and got "sent to
// everyone" would be a serious surprise on a live event.
func TestRSVPFollowUpRowsFilterByResponseIDRejectsNoMatchingRows(t *testing.T) {
	rows := []rsvpGuestRosterRow{
		{RSVPResponse: models.RSVPResponse{BaseModel: models.BaseModel{ID: uuid.New()}}},
	}
	staleID := uuid.New()

	filtered, err := filterRSVPFollowUpRowsByResponseID(rows, []string{staleID.String()})
	require.Error(t, err, "response_ids that match no row in the loaded audience must not silently fall back to the whole audience")
	assert.Nil(t, filtered)
	var userErr rsvpUserFacingError
	require.ErrorAs(t, err, &userErr, "must be a rsvpUserFacingError so SendRSVPFollowUp surfaces it as a 400 instead of a generic 500")
}
