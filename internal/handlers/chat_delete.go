package handlers

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/websocket"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

func (a *App) DeleteMessage(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	if !a.HasPermission(userID, models.ResourceContacts, models.ActionDelete, orgID) {
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, "You do not have permission to delete messages", nil, "")
	}

	contactID, err := parsePathUUID(r, "id", "contact")
	if err != nil {
		return nil
	}
	messageID, err := parsePathUUID(r, "message_id", "message")
	if err != nil {
		return nil
	}

	contact, err := findByIDAndOrg[models.Contact](a.DB, r, contactID, orgID, "Contact")
	if err != nil {
		return nil
	}

	var message models.Message
	if err := a.DB.Where("id = ? AND contact_id = ? AND organization_id = ?", messageID, contactID, orgID).First(&message).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Message not found", nil, "")
		}
		a.Log.Error("Failed to load message for deletion", "error", err, "message_id", messageID)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to delete message", nil, "")
	}

	tx := a.DB.Begin()
	if tx.Error != nil {
		a.Log.Error("Failed to start message delete transaction", "error", tx.Error)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to delete message", nil, "")
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	if err := tx.Model(&models.Message{}).
		Where("reply_to_message_id = ? AND deleted_at IS NULL", messageID).
		Updates(map[string]any{
			"reply_to_message_id": nil,
			"is_reply":            false,
		}).Error; err != nil {
		tx.Rollback()
		a.Log.Error("Failed to detach replies before message delete", "error", err, "message_id", messageID)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to delete message", nil, "")
	}

	if err := tx.Delete(&message).Error; err != nil {
		tx.Rollback()
		a.Log.Error("Failed to soft delete message", "error", err, "message_id", messageID)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to delete message", nil, "")
	}

	if err := a.refreshContactChatSnapshot(tx, contactID, time.Now().UTC()); err != nil {
		tx.Rollback()
		a.Log.Error("Failed to refresh contact after message delete", "error", err, "contact_id", contactID)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to delete message", nil, "")
	}

	if err := tx.Commit().Error; err != nil {
		a.Log.Error("Failed to commit message delete transaction", "error", err, "message_id", messageID)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to delete message", nil, "")
	}

	if err := a.DB.First(contact, contactID).Error; err != nil {
		a.Log.Error("Failed to reload contact after message delete", "error", err, "contact_id", contactID)
	}

	a.broadcastMessageDeleted(orgID, contactID, messageID, a.buildContactResponse(contact, orgID))

	return r.SendEnvelope(map[string]any{
		"message":    "Message deleted successfully",
		"message_id": messageID.String(),
		"contact":    a.buildContactResponse(contact, orgID),
	})
}

func (a *App) deleteContactConversation(tx *gorm.DB, contactID, orgID uuid.UUID) error {
	subquery := tx.Model(&models.Message{}).
		Select("id").
		Where("contact_id = ? AND organization_id = ? AND deleted_at IS NULL", contactID, orgID)

	if err := tx.Model(&models.Message{}).
		Where("reply_to_message_id IN (?) AND deleted_at IS NULL", subquery).
		Updates(map[string]any{
			"reply_to_message_id": nil,
			"is_reply":            false,
		}).Error; err != nil {
		return err
	}

	if err := tx.Where("contact_id = ? AND organization_id = ?", contactID, orgID).Delete(&models.Message{}).Error; err != nil {
		return err
	}

	return tx.Where("id = ? AND organization_id = ?", contactID, orgID).Delete(&models.Contact{}).Error
}

func (a *App) refreshContactChatSnapshot(tx *gorm.DB, contactID uuid.UUID, now time.Time) error {
	var latest models.Message
	latestErr := tx.Where("contact_id = ? AND deleted_at IS NULL", contactID).
		Order("created_at DESC, id DESC").
		First(&latest).Error

	var lastInbound sql.NullTime
	if err := tx.Model(&models.Message{}).
		Select("MAX(created_at)").
		Where("contact_id = ? AND direction = ? AND deleted_at IS NULL", contactID, models.DirectionIncoming).
		Scan(&lastInbound).Error; err != nil {
		return err
	}

	var unreadCount int64
	if err := tx.Model(&models.Message{}).
		Where("contact_id = ? AND direction = ? AND status != ? AND deleted_at IS NULL", contactID, models.DirectionIncoming, models.MessageStatusRead).
		Count(&unreadCount).Error; err != nil {
		return err
	}

	updates := map[string]any{
		"is_read":        unreadCount == 0,
		"last_inbound_at": nil,
		"updated_at":     now,
	}
	if lastInbound.Valid {
		updates["last_inbound_at"] = lastInbound.Time
	}

	if latestErr == nil {
		updates["last_message_at"] = latest.CreatedAt
		updates["last_message_preview"] = a.getPersistedMessagePreview(&latest)
		updates["whats_app_account"] = latest.WhatsAppAccount
	} else if latestErr == gorm.ErrRecordNotFound {
		updates["last_message_at"] = nil
		updates["last_message_preview"] = ""
	} else {
		return latestErr
	}

	return tx.Model(&models.Contact{}).Where("id = ?", contactID).Updates(updates).Error
}

func (a *App) broadcastMessageDeleted(orgID, contactID, messageID uuid.UUID, contact ContactResponse) {
	if a.WSHub == nil {
		return
	}

	a.WSHub.BroadcastToOrg(orgID, websocket.WSMessage{
		Type: websocket.TypeMessageDeleted,
		Payload: map[string]any{
			"contact_id":  contactID.String(),
			"message_id":  messageID.String(),
			"contact":     contact,
		},
	})
}

func (a *App) broadcastConversationDeleted(orgID, contactID uuid.UUID) {
	if a.WSHub == nil {
		return
	}

	a.WSHub.BroadcastToOrg(orgID, websocket.WSMessage{
		Type: websocket.TypeConversationDeleted,
		Payload: map[string]any{
			"contact_id": contactID.String(),
		},
	})
}
