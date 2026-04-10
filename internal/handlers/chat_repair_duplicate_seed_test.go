package handlers_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/require"
)

func seedChatRepairDuplicateMoveCandidates(t *testing.T, app *handlers.App, firstOrgID, secondOrgID, targetOrgID uuid.UUID) (*models.Contact, *models.Contact, *models.WhatsAppAccount) {
	t.Helper()

	firstAccount := &models.WhatsAppAccount{
		BaseModel:          models.BaseModel{ID: uuid.New()},
		OrganizationID:     firstOrgID,
		Name:               "duplicate-home-account-a",
		PhoneID:            "duplicate-home-phone-id-a",
		BusinessID:         "duplicate-home-business-id-a",
		AccessToken:        "test-token",
		WebhookVerifyToken: "verify-duplicate-home-a",
		APIVersion:         "v18.0",
		Status:             "active",
	}
	secondAccount := &models.WhatsAppAccount{
		BaseModel:          models.BaseModel{ID: uuid.New()},
		OrganizationID:     secondOrgID,
		Name:               "duplicate-home-account-b",
		PhoneID:            "duplicate-home-phone-id-b",
		BusinessID:         "duplicate-home-business-id-b",
		AccessToken:        "test-token",
		WebhookVerifyToken: "verify-duplicate-home-b",
		APIVersion:         "v18.0",
		Status:             "active",
	}
	targetAccount := &models.WhatsAppAccount{
		BaseModel:          models.BaseModel{ID: uuid.New()},
		OrganizationID:     targetOrgID,
		Name:               "duplicate-target-account",
		PhoneID:            "duplicate-target-phone-id",
		BusinessID:         "duplicate-target-business-id",
		AccessToken:        "test-token",
		WebhookVerifyToken: "verify-duplicate-target",
		APIVersion:         "v18.0",
		Status:             "active",
	}
	require.NoError(t, app.DB.Create(firstAccount).Error)
	require.NoError(t, app.DB.Create(secondAccount).Error)
	require.NoError(t, app.DB.Create(targetAccount).Error)

	sharedPhoneNumber := "919999000333"
	firstContact := testutil.CreateTestContactWith(
		t,
		app.DB,
		firstOrgID,
		testutil.WithContactAccount(firstAccount.Name),
		testutil.WithPhoneNumber(sharedPhoneNumber),
	)
	secondContact := testutil.CreateTestContactWith(
		t,
		app.DB,
		secondOrgID,
		testutil.WithContactAccount(secondAccount.Name),
		testutil.WithPhoneNumber(sharedPhoneNumber),
	)
	require.NoError(t, app.DB.Model(firstContact).Updates(map[string]any{
		"profile_name":         "Duplicate Move Contact A",
		"last_message_preview": "Wrong preview A",
		"whats_app_account":    firstAccount.Name,
	}).Error)
	require.NoError(t, app.DB.Model(secondContact).Updates(map[string]any{
		"profile_name":         "Duplicate Move Contact B",
		"last_message_preview": "Wrong preview B",
		"whats_app_account":    secondAccount.Name,
	}).Error)

	firstMessage := &models.Message{
		BaseModel:         models.BaseModel{ID: uuid.New()},
		OrganizationID:    firstOrgID,
		WhatsAppAccount:   firstAccount.Name,
		ContactID:         firstContact.ID,
		Direction:         models.DirectionOutgoing,
		MessageType:       models.MessageTypeText,
		Content:           "Duplicate move body A",
		Status:            models.MessageStatusSent,
		Metadata:          models.JSONB{"source": "external_api", "source_system": "aws_lambda", "phone_number_id": targetAccount.PhoneID},
		WhatsAppMessageID: "wamid.legacy-chat-repair-duplicate-a",
	}
	secondMessage := &models.Message{
		BaseModel:         models.BaseModel{ID: uuid.New()},
		OrganizationID:    secondOrgID,
		WhatsAppAccount:   secondAccount.Name,
		ContactID:         secondContact.ID,
		Direction:         models.DirectionOutgoing,
		MessageType:       models.MessageTypeText,
		Content:           "Duplicate move body B",
		Status:            models.MessageStatusSent,
		Metadata:          models.JSONB{"source": "external_api", "source_system": "aws_lambda", "phone_number_id": targetAccount.PhoneID},
		WhatsAppMessageID: "wamid.legacy-chat-repair-duplicate-b",
	}
	require.NoError(t, app.DB.Create(firstMessage).Error)
	require.NoError(t, app.DB.Create(secondMessage).Error)

	return firstContact, secondContact, targetAccount
}
