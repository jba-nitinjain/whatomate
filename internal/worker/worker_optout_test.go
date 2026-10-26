package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/internal/queue"
	"github.com/nikyjain/whatomate/pkg/whatsapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// optOutWorkerServer fails the test if the worker tries to send anything.
func optOutWorkerServer(t *testing.T, w *Worker, account *models.WhatsAppAccount) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		t.Errorf("worker must not send to an opted-out contact")
		rw.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	require.NoError(t, w.DB.Model(account).Update("api_version", "v21.0").Error)
	w.WhatsApp = whatsapp.NewWithBaseURL(w.Log, server.URL)
}

func recipientJobFor(org *models.Organization, campaign *models.BulkMessageCampaign, recipient *models.BulkMessageRecipient) *queue.RecipientJob {
	return &queue.RecipientJob{
		CampaignID:     campaign.ID,
		RecipientID:    recipient.ID,
		OrganizationID: org.ID,
		PhoneNumber:    recipient.PhoneNumber,
		RecipientName:  recipient.RecipientName,
		TemplateParams: recipient.TemplateParams,
	}
}

// Regression: the marketing opt-out skip path returned without checking
// campaign completion, leaving a campaign whose last recipient opted out
// stuck in "processing" forever.
func TestWorker_HandleRecipientJob_OptedOutLastRecipientCompletesCampaign(t *testing.T) {
	w := testWorker(t)
	org, account, _, campaign, recipient := createTestCampaignData(t, w)

	contact := &models.Contact{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		PhoneNumber:    recipient.PhoneNumber,
		ProfileName:    "Opted Out",
	}
	require.NoError(t, w.DB.Create(contact).Error)
	require.NoError(t, w.DB.Model(contact).Update("marketing_opt_out", true).Error)
	optOutWorkerServer(t, w, account)

	require.NoError(t, w.HandleRecipientJob(context.Background(), recipientJobFor(org, campaign, recipient)))

	require.NoError(t, w.DB.First(recipient, recipient.ID).Error)
	assert.Equal(t, models.MessageStatusFailed, recipient.Status)
	assert.Equal(t, "Contact opted out of marketing messages", recipient.ErrorMessage)

	require.NoError(t, w.DB.First(campaign, campaign.ID).Error)
	assert.Equal(t, 1, campaign.FailedCount)
	assert.Equal(t, models.CampaignStatusCompleted, campaign.Status)
	assert.NotNil(t, campaign.CompletedAt)
}

// The RSVP-linked reminder sync must still record the opt-out outcome on
// the RSVPReminderDelivery and finish the linked schedule.
func TestWorker_HandleRecipientJob_OptedOutRSVPReminderSyncsDelivery(t *testing.T) {
	w := testWorker(t)
	org, account, template, campaign, recipient := createTestCampaignData(t, w)
	schedule, delivery := createRSVPReminderDeliveryForRecipient(t, w, org, account, template, campaign, recipient, nil)
	require.NoError(t, w.DB.Model(&models.Contact{}).
		Where("organization_id = ? AND phone_number = ?", org.ID, recipient.PhoneNumber).
		Update("marketing_opt_out", true).Error)
	optOutWorkerServer(t, w, account)

	require.NoError(t, w.HandleRecipientJob(context.Background(), recipientJobFor(org, campaign, recipient)))

	require.NoError(t, w.DB.First(delivery, delivery.ID).Error)
	assert.Equal(t, models.RSVPReminderDeliveryFailed, delivery.Status)
	assert.Equal(t, "Contact opted out of marketing messages", delivery.ErrorMessage)

	require.NoError(t, w.DB.First(schedule, schedule.ID).Error)
	assert.Equal(t, models.RSVPReminderScheduleCompletedWithErrors, schedule.Status)
	assert.Equal(t, 1, schedule.FailedCount)
	assert.Zero(t, schedule.SentCount)

	require.NoError(t, w.DB.First(campaign, campaign.ID).Error)
	assert.Equal(t, models.CampaignStatusCompleted, campaign.Status)
}
