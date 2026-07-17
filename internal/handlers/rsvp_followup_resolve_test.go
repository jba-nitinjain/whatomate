package handlers

import (
	"testing"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRSVPEventForFlowOrFollowUp proves rsvpEventForFlowOrFollowUp resolves an
// active RSVP event by either of the two routes a flow can reach one through:
// being the event's own primary flow, or being the flow a follow-up campaign
// for that event hands the guest.
//
// The second route is the entire reason this function exists. A follow-up
// runs a DIFFERENT flow than the event's primary flow (Task 5 rejects the
// event's primary flow as a follow-up flow), so the old single-route
// rsvpEventForFlow - which only matches rsvp_events.flow_id - always returns
// nil for a follow-up tap-through: the RSVP hook in chatbot_processor.go is
// skipped, _rsvp_event_id is never set on the session, and
// finalizeRSVPFromSession returns early. The guest's answer is silently
// discarded, with every other test green.
func TestRSVPEventForFlowOrFollowUp(t *testing.T) {
	db := testutil.SetupTestDB(t)
	app := &App{DB: db, Log: testutil.NopLogger()}

	t.Run("primary flow resolves directly, not as a follow-up", func(t *testing.T) {
		org := testutil.CreateTestOrganization(t, db)
		flowA := uuid.New()
		event := models.RSVPEvent{
			BaseModel:      models.BaseModel{ID: uuid.New()},
			OrganizationID: org.ID,
			Name:           "Primary Flow Event",
			Status:         models.RSVPEventStatusActive,
			FlowID:         &flowA,
			CreatedBy:      uuid.New(),
		}
		require.NoError(t, db.Create(&event).Error)

		got, isFollowUp := app.rsvpEventForFlowOrFollowUp(org.ID, flowA)
		require.NotNil(t, got)
		assert.Equal(t, event.ID, got.ID)
		assert.False(t, isFollowUp, "the event's own flow must not be reported as a follow-up")
	})

	t.Run("unrelated flow resolves to nothing - the regression this task exists to prevent", func(t *testing.T) {
		org := testutil.CreateTestOrganization(t, db)
		flowA := uuid.New()
		flowB := uuid.New()
		event := models.RSVPEvent{
			BaseModel:      models.BaseModel{ID: uuid.New()},
			OrganizationID: org.ID,
			Name:           "Unrelated Flow Event",
			Status:         models.RSVPEventStatusActive,
			FlowID:         &flowA,
			CreatedBy:      uuid.New(),
		}
		require.NoError(t, db.Create(&event).Error)

		// flowB is not linked to anything yet (no campaign row exists for it).
		// If this ever resolves to non-nil here, or a caller ever treats it as
		// a follow-up, a real follow-up tap-through's answer would be silently
		// discarded: finalizeRSVPFromSession never runs because
		// _rsvp_event_id is never set on the session.
		got, isFollowUp := app.rsvpEventForFlowOrFollowUp(org.ID, flowB)
		assert.Nil(t, got, "a flow with no campaign link must resolve to no event")
		assert.False(t, isFollowUp)
	})

	t.Run("follow-up campaign flow resolves to the event as a follow-up", func(t *testing.T) {
		org := testutil.CreateTestOrganization(t, db)
		flowA := uuid.New()
		flowB := uuid.New()
		event := models.RSVPEvent{
			BaseModel:      models.BaseModel{ID: uuid.New()},
			OrganizationID: org.ID,
			Name:           "Follow-up Flow Event",
			Status:         models.RSVPEventStatusActive,
			FlowID:         &flowA,
			CreatedBy:      uuid.New(),
		}
		require.NoError(t, db.Create(&event).Error)

		template := testutil.CreateTestTemplate(t, db, org.ID, "test-account")
		user := testutil.CreateTestUser(t, db, org.ID)
		campaign := models.BulkMessageCampaign{
			BaseModel:       models.BaseModel{ID: uuid.New()},
			OrganizationID:  org.ID,
			WhatsAppAccount: "test-account",
			Name:            "Follow-up Campaign",
			TemplateID:      template.ID,
			CreatedBy:       user.ID,
			SourceType:      models.CampaignSourceRSVPFollowUp,
			SourceID:        &event.ID,
			FlowID:          &flowB,
		}
		require.NoError(t, db.Create(&campaign).Error)

		got, isFollowUp := app.rsvpEventForFlowOrFollowUp(org.ID, flowB)
		require.NotNil(t, got, "a flow linked via a follow-up campaign must resolve to the event")
		assert.Equal(t, event.ID, got.ID)
		assert.True(t, isFollowUp, "a flow reached via a follow-up campaign must be reported as a follow-up")
	})

	t.Run("reminder campaign does not count as a follow-up flow link", func(t *testing.T) {
		org := testutil.CreateTestOrganization(t, db)
		flowA := uuid.New()
		flowB := uuid.New()
		event := models.RSVPEvent{
			BaseModel:      models.BaseModel{ID: uuid.New()},
			OrganizationID: org.ID,
			Name:           "Reminder Only Event",
			Status:         models.RSVPEventStatusActive,
			FlowID:         &flowA,
			CreatedBy:      uuid.New(),
		}
		require.NoError(t, db.Create(&event).Error)

		// A reminder campaign does not run a flow (it nudges people to respond
		// via WhatsApp, it does not hand them into a chatbot flow), so it must
		// never be mistaken for a follow-up flow link even though it points at
		// the same flow id.
		template := testutil.CreateTestTemplate(t, db, org.ID, "test-account")
		user := testutil.CreateTestUser(t, db, org.ID)
		campaign := models.BulkMessageCampaign{
			BaseModel:       models.BaseModel{ID: uuid.New()},
			OrganizationID:  org.ID,
			WhatsAppAccount: "test-account",
			Name:            "Reminder Campaign",
			TemplateID:      template.ID,
			CreatedBy:       user.ID,
			SourceType:      models.CampaignSourceRSVPReminder,
			SourceID:        &event.ID,
			FlowID:          &flowB,
		}
		require.NoError(t, db.Create(&campaign).Error)

		got, isFollowUp := app.rsvpEventForFlowOrFollowUp(org.ID, flowB)
		assert.Nil(t, got, "a reminder campaign does not run a flow, so it must not resolve a flow to an event")
		assert.False(t, isFollowUp)
	})

	t.Run("closed event does not resolve by either route", func(t *testing.T) {
		org := testutil.CreateTestOrganization(t, db)
		flowA := uuid.New()
		flowB := uuid.New()
		event := models.RSVPEvent{
			BaseModel:      models.BaseModel{ID: uuid.New()},
			OrganizationID: org.ID,
			Name:           "Closed Event",
			Status:         models.RSVPEventStatusClosed,
			FlowID:         &flowA,
			CreatedBy:      uuid.New(),
		}
		require.NoError(t, db.Create(&event).Error)

		template := testutil.CreateTestTemplate(t, db, org.ID, "test-account")
		user := testutil.CreateTestUser(t, db, org.ID)
		campaign := models.BulkMessageCampaign{
			BaseModel:       models.BaseModel{ID: uuid.New()},
			OrganizationID:  org.ID,
			WhatsAppAccount: "test-account",
			Name:            "Follow-up Campaign On Closed Event",
			TemplateID:      template.ID,
			CreatedBy:       user.ID,
			SourceType:      models.CampaignSourceRSVPFollowUp,
			SourceID:        &event.ID,
			FlowID:          &flowB,
		}
		require.NoError(t, db.Create(&campaign).Error)

		gotPrimary, isFollowUpPrimary := app.rsvpEventForFlowOrFollowUp(org.ID, flowA)
		assert.Nil(t, gotPrimary, "a closed event must not resolve via its primary flow")
		assert.False(t, isFollowUpPrimary)

		gotFollowUp, isFollowUp := app.rsvpEventForFlowOrFollowUp(org.ID, flowB)
		assert.Nil(t, gotFollowUp, "a closed event must not resolve via a follow-up campaign flow either")
		assert.False(t, isFollowUp)
	})

	t.Run("another org's campaign does not resolve", func(t *testing.T) {
		orgA := testutil.CreateTestOrganization(t, db)
		orgB := testutil.CreateTestOrganization(t, db)
		flowA := uuid.New()
		flowB := uuid.New()
		event := models.RSVPEvent{
			BaseModel:      models.BaseModel{ID: uuid.New()},
			OrganizationID: orgA.ID,
			Name:           "Org A Event",
			Status:         models.RSVPEventStatusActive,
			FlowID:         &flowA,
			CreatedBy:      uuid.New(),
		}
		require.NoError(t, db.Create(&event).Error)

		// Campaign belongs to org B and points its SourceID at org A's event -
		// this must never let org B's flow resolve org A's event.
		template := testutil.CreateTestTemplate(t, db, orgB.ID, "test-account")
		user := testutil.CreateTestUser(t, db, orgB.ID)
		campaign := models.BulkMessageCampaign{
			BaseModel:       models.BaseModel{ID: uuid.New()},
			OrganizationID:  orgB.ID,
			WhatsAppAccount: "test-account",
			Name:            "Cross-org Campaign",
			TemplateID:      template.ID,
			CreatedBy:       user.ID,
			SourceType:      models.CampaignSourceRSVPFollowUp,
			SourceID:        &event.ID,
			FlowID:          &flowB,
		}
		require.NoError(t, db.Create(&campaign).Error)

		got, isFollowUp := app.rsvpEventForFlowOrFollowUp(orgB.ID, flowB)
		assert.Nil(t, got, "a campaign in another org must not resolve an event that belongs to a different org")
		assert.False(t, isFollowUp)
	})
}
