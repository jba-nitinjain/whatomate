package handlers_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/handlers"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/require"
)

func seedChatRepairCandidate(t *testing.T, app *handlers.App, homeOrgID, targetOrgID uuid.UUID) (*models.Contact, *models.WhatsAppAccount) {
	t.Helper()

	homeAccount := &models.WhatsAppAccount{
		BaseModel:          models.BaseModel{ID: uuid.New()},
		OrganizationID:     homeOrgID,
		Name:               "home-account",
		PhoneID:            "home-phone-id",
		BusinessID:         "home-business-id",
		AccessToken:        "test-token",
		WebhookVerifyToken: "verify-home",
		APIVersion:         "v18.0",
		Status:             "active",
	}
	targetAccount := &models.WhatsAppAccount{
		BaseModel:          models.BaseModel{ID: uuid.New()},
		OrganizationID:     targetOrgID,
		Name:               "target-account",
		PhoneID:            "target-phone-id",
		BusinessID:         "target-business-id",
		AccessToken:        "test-token",
		WebhookVerifyToken: "verify-target",
		APIVersion:         "v18.0",
		Status:             "active",
	}
	require.NoError(t, app.DB.Create(homeAccount).Error)
	require.NoError(t, app.DB.Create(targetAccount).Error)

	wrongContact := testutil.CreateTestContactWith(
		t,
		app.DB,
		homeOrgID,
		testutil.WithContactAccount(homeAccount.Name),
		testutil.WithPhoneNumber("919999000111"),
	)
	require.NoError(t, app.DB.Model(wrongContact).Updates(map[string]any{
		"profile_name":         "Legacy Contact",
		"last_message_preview": "Wrong preview",
		"whats_app_account":    homeAccount.Name,
	}).Error)

	message := &models.Message{
		BaseModel:         models.BaseModel{ID: uuid.New()},
		OrganizationID:    homeOrgID,
		WhatsAppAccount:   homeAccount.Name,
		ContactID:         wrongContact.ID,
		Direction:         models.DirectionOutgoing,
		MessageType:       models.MessageTypeText,
		Content:           "Legacy sync body",
		Status:            models.MessageStatusSent,
		Metadata:          models.JSONB{"source": "external_api", "source_system": "aws_lambda", "phone_number_id": targetAccount.PhoneID},
		WhatsAppMessageID: "wamid.legacy-chat-repair",
	}
	require.NoError(t, app.DB.Create(message).Error)

	return wrongContact, targetAccount
}

func seedChatRepairMergeCandidate(t *testing.T, app *handlers.App, homeOrgID, targetOrgID uuid.UUID) (*models.Contact, *models.Contact, *models.WhatsAppAccount) {
	t.Helper()

	wrongContact, targetAccount := seedChatRepairCandidate(t, app, homeOrgID, targetOrgID)
	targetContact := testutil.CreateTestContactWith(
		t,
		app.DB,
		targetOrgID,
		testutil.WithContactAccount(targetAccount.Name),
		testutil.WithPhoneNumber(wrongContact.PhoneNumber),
	)
	require.NoError(t, app.DB.Model(targetContact).Updates(map[string]any{
		"profile_name":         "Correct Contact",
		"last_message_preview": "Correct preview",
		"whats_app_account":    targetAccount.Name,
	}).Error)
	require.NoError(t, app.DB.Model(&models.Message{}).Where("contact_id = ?", wrongContact.ID).Update("whats_app_message_id", "wamid.legacy-chat-repair-merge").Error)

	return wrongContact, targetContact, targetAccount
}

func seedChatRepairDeletedTargetCandidate(t *testing.T, app *handlers.App, homeOrgID, targetOrgID uuid.UUID) (*models.Contact, *models.Contact, *models.WhatsAppAccount) {
	t.Helper()

	wrongContact, targetAccount := seedChatRepairCandidate(t, app, homeOrgID, targetOrgID)
	targetContact := testutil.CreateTestContactWith(
		t,
		app.DB,
		targetOrgID,
		testutil.WithContactAccount(targetAccount.Name),
		testutil.WithPhoneNumber(wrongContact.PhoneNumber),
	)
	require.NoError(t, app.DB.Model(targetContact).Updates(map[string]any{
		"profile_name":         "Deleted Target Contact",
		"last_message_preview": "Deleted preview",
		"whats_app_account":    targetAccount.Name,
	}).Error)
	require.NoError(t, app.DB.Delete(targetContact).Error)
	require.NoError(t, app.DB.Model(&models.Message{}).Where("contact_id = ?", wrongContact.ID).Update("whats_app_message_id", "wamid.legacy-chat-repair-deleted-target").Error)

	return wrongContact, targetContact, targetAccount
}

func seedChatRepairAccountEvidenceCandidate(t *testing.T, app *handlers.App, homeOrgID, targetOrgID uuid.UUID) (*models.Contact, *models.WhatsAppAccount) {
	t.Helper()

	homeAccount := &models.WhatsAppAccount{
		BaseModel:          models.BaseModel{ID: uuid.New()},
		OrganizationID:     homeOrgID,
		Name:               "home-account",
		PhoneID:            "home-phone-id-account-evidence",
		BusinessID:         "home-business-id-account-evidence",
		AccessToken:        "test-token",
		WebhookVerifyToken: "verify-home-account-evidence",
		APIVersion:         "v18.0",
		Status:             "active",
	}
	targetAccount := &models.WhatsAppAccount{
		BaseModel:          models.BaseModel{ID: uuid.New()},
		OrganizationID:     targetOrgID,
		Name:               "target-account-account-evidence",
		PhoneID:            "target-phone-id-account-evidence",
		BusinessID:         "target-business-id-account-evidence",
		AccessToken:        "test-token",
		WebhookVerifyToken: "verify-target-account-evidence",
		APIVersion:         "v18.0",
		Status:             "active",
	}
	require.NoError(t, app.DB.Create(homeAccount).Error)
	require.NoError(t, app.DB.Create(targetAccount).Error)

	wrongContact := testutil.CreateTestContactWith(
		t,
		app.DB,
		homeOrgID,
		testutil.WithContactAccount(homeAccount.Name),
		testutil.WithPhoneNumber("919999000222"),
	)
	require.NoError(t, app.DB.Model(wrongContact).Updates(map[string]any{
		"profile_name":         "Account Evidence Contact",
		"last_message_preview": "Wrong preview",
		"whats_app_account":    targetAccount.Name,
	}).Error)

	message := &models.Message{
		BaseModel:         models.BaseModel{ID: uuid.New()},
		OrganizationID:    homeOrgID,
		WhatsAppAccount:   targetAccount.Name,
		ContactID:         wrongContact.ID,
		Direction:         models.DirectionOutgoing,
		MessageType:       models.MessageTypeText,
		Content:           "Account evidence body",
		Status:            models.MessageStatusSent,
		Metadata:          models.JSONB{"source": "external_api"},
		WhatsAppMessageID: "wamid.account-evidence-chat-repair",
	}
	require.NoError(t, app.DB.Create(message).Error)

	return wrongContact, targetAccount
}
