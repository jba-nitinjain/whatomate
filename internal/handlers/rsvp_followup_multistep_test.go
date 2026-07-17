package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStartFlow_FollowUp_MultiStep_FlagSurvivesEveryStepAndMergesOnCompletion
// is the highest-risk proof in this task: the follow-up flag set at
// startFlow (chatbot_processor.go:872-880) must survive not just the first
// step, but EVERY step of a multi-step flow, all the way to the step where
// the flow actually completes - because that is where finalizeRSVPFromSession
// runs the merge that protects 271 members' and 207 spouses' existing
// attendance/spouse answers from being clobbered.
//
// A real WhatsApp conversation is not one function call: each guest reply
// arrives as its own webhook request, and getOrCreateSession
// (chatbot_processor.go:772-809) reloads the session fresh from Postgres
// every time. So this test reloads the session from the DB between every
// step - exactly like production - rather than reusing the in-memory
// session.SessionData map, which would prove nothing about persistence.
func TestStartFlow_FollowUp_MultiStep_FlagSurvivesEveryStepAndMergesOnCompletion(t *testing.T) {
	app := newProcessorTestApp(t)
	org, account := createProcessorTestOrg(t, app)

	// The event's own primary flow (not exercised in this test - only its id
	// matters, so rsvpEventForFlowOrFollowUp's primary-flow route does not
	// accidentally match the follow-up flow below).
	primaryFlowID := uuid.New()

	// The follow-up flow: two steps, so the flag must survive from step 1's
	// answer all the way to step 2's completion.
	followUpFlowID := uuid.New()
	followUpFlow := &models.ChatbotFlow{
		BaseModel:       models.BaseModel{ID: followUpFlowID},
		OrganizationID:  org.ID,
		WhatsAppAccount: account.Name,
		Name:            "Children Follow-up Flow",
		IsEnabled:       true,
		Steps: []models.ChatbotFlowStep{
			{
				BaseModel:   models.BaseModel{ID: uuid.New()},
				FlowID:      followUpFlowID,
				StepName:    "children_step",
				StepOrder:   1,
				Message:     "How many children are coming?",
				MessageType: models.FlowStepTypeText,
				InputType:   models.InputTypeText,
				StoreAs:     "children_count",
				NextStep:    "notes_step",
			},
			{
				BaseModel:   models.BaseModel{ID: uuid.New()},
				FlowID:      followUpFlowID,
				StepName:    "notes_step",
				StepOrder:   2,
				Message:     "Any dietary notes for the children?",
				MessageType: models.FlowStepTypeText,
				InputType:   models.InputTypeText,
				StoreAs:     "notes",
			},
		},
	}
	require.NoError(t, app.DB.Create(followUpFlow).Error)

	event := &models.RSVPEvent{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		Name:            "Live Event 19/07/2026",
		Status:          models.RSVPEventStatusActive,
		WhatsAppAccount: account.Name,
		FlowID:          &primaryFlowID,
		AttendanceField: "attendance",
		CreatedBy:       uuid.New(),
	}
	require.NoError(t, app.DB.Create(event).Error)

	// Task 5's durable link: a follow-up campaign recording which flow a
	// tap-through hands the guest into. This - not anything carried on the
	// session - is what rsvpEventForFlowOrFollowUp resolves through.
	template := testutil.CreateTestTemplate(t, app.DB, org.ID, account.Name)
	campaignCreator := testutil.CreateTestUser(t, app.DB, org.ID)
	campaign := &models.BulkMessageCampaign{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		WhatsAppAccount: account.Name,
		Name:            "Children Follow-up Campaign",
		TemplateID:      template.ID,
		CreatedBy:       campaignCreator.ID,
		SourceType:      models.CampaignSourceRSVPFollowUp,
		SourceID:        &event.ID,
		FlowID:          &followUpFlowID,
	}
	require.NoError(t, app.DB.Create(campaign).Error)

	phone := "919840099099"
	contact := testutil.CreateTestContactWith(t, app.DB, org.ID,
		testutil.WithContactAccount(account.Name), testutil.WithPhoneNumber(phone))

	// This guest already responded days ago - attendance and spouse answers
	// are live data that must survive the follow-up untouched.
	respondedAt := time.Now().UTC().AddDate(0, 0, -2)
	existingResponse := &models.RSVPResponse{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		RSVPEventID:    event.ID,
		OrganizationID: org.ID,
		ContactID:      contact.ID,
		PhoneNumber:    phone,
		Attendance:     models.RSVPAttendanceYes,
		Answers: models.JSONB{
			"attendance":              "yes",
			"attendance_title":        "Attending",
			"spouse_attendance":       "yes",
			"spouse_attendance_title": "Attending",
			"spouse_mobile":           "919840026099",
		},
		RespondedAt: &respondedAt,
		Source:      models.RSVPGuestSourceContactSelection,
	}
	require.NoError(t, app.DB.Create(existingResponse).Error)

	// --- Step 0: guest taps the follow-up link, startFlow runs. ---
	session := &models.ChatbotSession{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		ContactID:       contact.ID,
		WhatsAppAccount: account.Name,
		PhoneNumber:     phone,
		Status:          models.SessionStatusActive,
		SessionData:     models.JSONB{},
		StartedAt:       time.Now(),
		LastActivityAt:  time.Now(),
	}
	require.NoError(t, app.DB.Create(session).Error)

	app.startFlow(account, session, contact, followUpFlow, "", "")

	// --- Reload from DB exactly as a fresh webhook request for the guest's
	// reply to step 1 would (getOrCreateSession re-fetches by row, it never
	// reuses an in-memory session). This is the proof the flag survived the
	// FIRST DB round-trip, not just the in-memory struct startFlow already
	// held. ---
	var afterStart models.ChatbotSession
	require.NoError(t, app.DB.First(&afterStart, "id = ?", session.ID).Error)
	assert.Equal(t, "children_step", afterStart.CurrentStep,
		"the follow-up flow must advance to its first step, exactly like a normal flow")
	flag, isBool := afterStart.SessionData[rsvpFollowUpKey].(bool)
	require.True(t, isBool, "the follow-up flag must round-trip through Postgres as a bool")
	assert.True(t, flag, "the follow-up flag must be set after startFlow, before any step reply")
	assert.Equal(t, event.ID.String(), afterStart.SessionData[rsvpEventIDKey],
		"the session must be tagged with the resolved event, not just the follow-up flag")

	// --- Step 1: guest answers "how many children" - a NEW request reloading
	// the session from DB, mirroring production. ---
	app.processFlowResponse(account, &afterStart, contact, "2", "", nil)

	// Incremental finalize (chatbot_processor.go:1243) must have already
	// merged children_count in WITHOUT touching the pre-existing answers -
	// proving the flag was read correctly for a mid-flow step, not just the
	// first one.
	var afterStep1Response models.RSVPResponse
	require.NoError(t, app.DB.First(&afterStep1Response, "rsvp_event_id = ? AND contact_id = ?", event.ID, contact.ID).Error)
	assert.Equal(t, models.RSVPAttendanceYes, afterStep1Response.Attendance,
		"a mid-flow follow-up step must not recompute attendance")
	assert.Equal(t, "yes", afterStep1Response.Answers["spouse_attendance"],
		"a mid-flow follow-up step must not erase spouse_attendance")
	assert.Equal(t, "2", afterStep1Response.Answers["children_count"],
		"a mid-flow follow-up step must add its answer")
	require.NotNil(t, afterStep1Response.RespondedAt)
	assert.WithinDuration(t, respondedAt, *afterStep1Response.RespondedAt, time.Second,
		"a mid-flow follow-up step must not move responded_at")

	// --- Reload again from DB, mirroring the NEXT separate webhook request
	// for the guest's reply to step 2. This is the proof the flag survives
	// past the first step, not just into it. ---
	var afterStep1Session models.ChatbotSession
	require.NoError(t, app.DB.First(&afterStep1Session, "id = ?", session.ID).Error)
	assert.Equal(t, "notes_step", afterStep1Session.CurrentStep,
		"the flow must have advanced to its second step")
	flag2, isBool2 := afterStep1Session.SessionData[rsvpFollowUpKey].(bool)
	require.True(t, isBool2, "the follow-up flag must still be a bool after surviving to step 2")
	assert.True(t, flag2, "the follow-up flag must still be true going into the FINAL step")

	// --- Step 2 (final): guest answers the last question, the flow
	// completes, and finalizeRSVPFromSession runs from completeFlow. This is
	// the assertion the brief calls out explicitly: the merge must still
	// happen on the FINAL step, not just the first. ---
	app.processFlowResponse(account, &afterStep1Session, contact, "None", "", nil)

	var final models.RSVPResponse
	require.NoError(t, app.DB.First(&final, "rsvp_event_id = ? AND contact_id = ?", event.ID, contact.ID).Error)
	assert.Equal(t, models.RSVPAttendanceYes, final.Attendance,
		"completion of a multi-step follow-up must not recompute attendance - a live record would be corrupted")
	assert.Equal(t, "yes", final.Answers["spouse_attendance"],
		"completion of a multi-step follow-up must not erase spouse_attendance - a live record would be corrupted")
	assert.Equal(t, "Attending", final.Answers["spouse_attendance_title"],
		"completion of a multi-step follow-up must not erase spouse_attendance_title - a live record would be corrupted")
	assert.Equal(t, "919840026099", final.Answers["spouse_mobile"],
		"completion of a multi-step follow-up must not erase spouse_mobile - a live record would be corrupted")
	assert.Equal(t, "Attending", final.Answers["attendance_title"],
		"completion of a multi-step follow-up must not erase attendance_title - a live record would be corrupted")
	assert.Equal(t, "2", final.Answers["children_count"],
		"the answer from step 1 must survive to the final merged record")
	assert.Equal(t, "None", final.Answers["notes"],
		"the answer from the FINAL step must be captured - this is the merge the flag exists to enable")
	require.NotNil(t, final.RespondedAt)
	assert.WithinDuration(t, respondedAt, *final.RespondedAt, time.Second,
		"completing a follow-up must not move responded_at - the guest originally answered days ago")

	var finalSession models.ChatbotSession
	require.NoError(t, app.DB.First(&finalSession, "id = ?", session.ID).Error)
	assert.Equal(t, models.SessionStatusCompleted, finalSession.Status,
		"the follow-up flow must complete normally once its last step is answered")
}
