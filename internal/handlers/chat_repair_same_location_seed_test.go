package handlers_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/require"
)

func seedChatRepairSameLocationMessageDriftCandidate(t *testing.T, app *handlers.App, targetOrgID, driftOrgID uuid.UUID) (*models.Contact, *models.WhatsAppAccount) {
	t.Helper()

	targetAccount := &models.WhatsAppAccount{
		BaseModel:          models.BaseModel{ID: uuid.New()},
		OrganizationID:     targetOrgID,
		Name:               "same-location-target-account",
		PhoneID:            "same-location-target-phone-id",
		BusinessID:         "same-location-target-business-id",
		AccessToken:        "test-token",
		WebhookVerifyToken: "verify-same-location-target",
		APIVersion:         "v18.0",
		Status:             "active",
	}
	require.NoError(t, app.DB.Create(targetAccount).Error)

	contact := testutil.CreateTestContactWith(
		t,
		app.DB,
		targetOrgID,
		testutil.WithContactAccount(targetAccount.Name),
		testutil.WithPhoneNumber("919999000444"),
	)
	require.NoError(t, app.DB.Model(contact).Updates(map[string]any{
		"profile_name":         "Same Location Drift Contact",
		"last_message_preview": "Aligned preview",
		"whats_app_account":    targetAccount.Name,
	}).Error)

	message := &models.Message{
		BaseModel:         models.BaseModel{ID: uuid.New()},
		OrganizationID:    driftOrgID,
		WhatsAppAccount:   "legacy-drift-account",
		ContactID:         contact.ID,
		Direction:         models.DirectionIncoming,
		MessageType:       models.MessageTypeText,
		Content:           "Drifted message body",
		Status:            models.MessageStatusDelivered,
		Metadata:          models.JSONB{"source": "external_api", "source_system": "aws_lambda", "phone_number_id": targetAccount.PhoneID},
		WhatsAppMessageID: "wamid.same-location-message-drift",
	}
	require.NoError(t, app.DB.Create(message).Error)

	return contact, targetAccount
}

func seedChatRepairSameLocationAlignedCandidate(t *testing.T, app *handlers.App, targetOrgID uuid.UUID) (*models.Contact, *models.WhatsAppAccount) {
	t.Helper()

	targetAccount := &models.WhatsAppAccount{
		BaseModel:          models.BaseModel{ID: uuid.New()},
		OrganizationID:     targetOrgID,
		Name:               "same-location-aligned-account",
		PhoneID:            "same-location-aligned-phone-id",
		BusinessID:         "same-location-aligned-business-id",
		AccessToken:        "test-token",
		WebhookVerifyToken: "verify-same-location-aligned",
		APIVersion:         "v18.0",
		Status:             "active",
	}
	require.NoError(t, app.DB.Create(targetAccount).Error)

	contact := testutil.CreateTestContactWith(
		t,
		app.DB,
		targetOrgID,
		testutil.WithContactAccount(targetAccount.Name),
		testutil.WithPhoneNumber("919999000555"),
	)
	require.NoError(t, app.DB.Model(contact).Updates(map[string]any{
		"profile_name":         "Same Location Aligned Contact",
		"last_message_preview": "Aligned preview",
		"whats_app_account":    targetAccount.Name,
	}).Error)

	message := &models.Message{
		BaseModel:         models.BaseModel{ID: uuid.New()},
		OrganizationID:    targetOrgID,
		WhatsAppAccount:   targetAccount.Name,
		ContactID:         contact.ID,
		Direction:         models.DirectionIncoming,
		MessageType:       models.MessageTypeText,
		Content:           "Aligned message body",
		Status:            models.MessageStatusDelivered,
		Metadata:          models.JSONB{"source": "external_api", "source_system": "aws_lambda", "phone_number_id": targetAccount.PhoneID},
		WhatsAppMessageID: "wamid.same-location-aligned",
	}
	require.NoError(t, app.DB.Create(message).Error)

	return contact, targetAccount
}
