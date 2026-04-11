package handlers

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

// OrgMismatchRecord describes a contact whose organization_id does not match
// the organisation that owns the phone_number_id stored in its metadata.
type OrgMismatchRecord struct {
	ContactID      string `json:"contact_id"`
	PhoneNumber    string `json:"phone_number"`
	ProfileName    string `json:"profile_name"`
	CurrentOrgID   string `json:"current_org_id"`
	CurrentOrgName string `json:"current_org_name"`
	CorrectOrgID   string `json:"correct_org_id"`
	CorrectOrgName string `json:"correct_org_name"`
	PhoneNumberID  string `json:"phone_number_id"`
	MessageCount   int    `json:"message_count"`
}

// OrgMismatchPreviewResponse is the response from the preview endpoint.
type OrgMismatchPreviewResponse struct {
	Count   int                  `json:"count"`
	Records []OrgMismatchRecord  `json:"records"`
}

// OrgMismatchApplyResponse summarises the result of applying the fix.
type OrgMismatchApplyResponse struct {
	Fixed  int      `json:"fixed"`
	Errors []string `json:"errors,omitempty"`
}

// PreviewOrgMismatch scans contacts whose phone_number_id in metadata belongs to a
// different organisation than the contact's own organization_id.
// Only accessible by super admins.
func (a *App) PreviewOrgMismatch(r *fastglue.Request) error {
	userID, ok := r.RequestCtx.UserValue("user_id").(uuid.UUID)
	if !ok {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	if !a.IsSuperAdmin(userID) {
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, "Only super admins can access org mismatch repair", nil, "")
	}

	records, err := a.findOrgMismatchedContacts()
	if err != nil {
		a.Log.Error("Failed to scan org-mismatched contacts", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to scan for mismatched records", nil, "")
	}

	return r.SendEnvelope(OrgMismatchPreviewResponse{
		Count:   len(records),
		Records: records,
	})
}

// ApplyOrgMismatchFix moves each mismatched contact (and all its messages) to the
// correct organisation as determined by the phone_number_id in its metadata.
// Only accessible by super admins.
func (a *App) ApplyOrgMismatchFix(r *fastglue.Request) error {
	userID, ok := r.RequestCtx.UserValue("user_id").(uuid.UUID)
	if !ok {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	if !a.IsSuperAdmin(userID) {
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, "Only super admins can access org mismatch repair", nil, "")
	}

	records, err := a.findOrgMismatchedContacts()
	if err != nil {
		a.Log.Error("Failed to scan org-mismatched contacts for apply", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to scan for mismatched records", nil, "")
	}

	if len(records) == 0 {
		return r.SendEnvelope(OrgMismatchApplyResponse{Fixed: 0})
	}

	fixed := 0
	var errs []string

	for _, rec := range records {
		contactID, err := uuid.Parse(rec.ContactID)
		if err != nil {
			errs = append(errs, fmt.Sprintf("invalid contact_id %s: %v", rec.ContactID, err))
			continue
		}
		correctOrgID, err := uuid.Parse(rec.CorrectOrgID)
		if err != nil {
			errs = append(errs, fmt.Sprintf("invalid correct_org_id %s: %v", rec.CorrectOrgID, err))
			continue
		}

		// Determine the WhatsApp account name in the target org for this phone_number_id.
		var targetAccount models.WhatsAppAccount
		if dbErr := a.DB.Where("organization_id = ? AND phone_id = ?", correctOrgID, rec.PhoneNumberID).
			First(&targetAccount).Error; dbErr != nil {
			errs = append(errs, fmt.Sprintf("contact %s: cannot find target account: %v", rec.ContactID, dbErr))
			continue
		}

		// Check if a contact with the same phone number already exists in the target org.
		var existingInTarget models.Contact
		alreadyExists := false
		if dbErr := a.DB.Where("organization_id = ? AND phone_number = ?", correctOrgID, rec.PhoneNumber).
			First(&existingInTarget).Error; dbErr == nil {
			alreadyExists = true
		}

		if alreadyExists {
			// Merge: move messages from wrong contact to existing contact, then delete wrong contact.
			if txErr := a.DB.Transaction(func(tx *gorm.DB) error {
				if err := tx.Model(&models.Message{}).
					Where("contact_id = ?", contactID).
					Updates(map[string]interface{}{
						"contact_id":      existingInTarget.ID,
						"organization_id": correctOrgID,
						"whats_app_account": targetAccount.Name,
					}).Error; err != nil {
					return err
				}
				// Soft-delete the orphaned contact.
				return tx.Delete(&models.Contact{}, "id = ?", contactID).Error
			}); txErr != nil {
				errs = append(errs, fmt.Sprintf("contact %s merge failed: %v", rec.ContactID, txErr))
				continue
			}
		} else {
			// Move: update organization_id of contact and all its messages.
			if txErr := a.DB.Transaction(func(tx *gorm.DB) error {
				if err := tx.Model(&models.Contact{}).
					Where("id = ?", contactID).
					Updates(map[string]interface{}{
						"organization_id":  correctOrgID,
						"whats_app_account": targetAccount.Name,
					}).Error; err != nil {
					return err
				}
				return tx.Model(&models.Message{}).
					Where("contact_id = ?", contactID).
					Updates(map[string]interface{}{
						"organization_id":   correctOrgID,
						"whats_app_account": targetAccount.Name,
					}).Error
			}); txErr != nil {
				errs = append(errs, fmt.Sprintf("contact %s move failed: %v", rec.ContactID, txErr))
				continue
			}
		}

		fixed++
		a.Log.Info("Fixed org-mismatched contact",
			"contact_id", rec.ContactID,
			"from_org", rec.CurrentOrgID,
			"to_org", rec.CorrectOrgID,
			"merged", alreadyExists,
		)
	}

	return r.SendEnvelope(OrgMismatchApplyResponse{
		Fixed:  fixed,
		Errors: errs,
	})
}

// findOrgMismatchedContacts returns contacts whose metadata phone_number_id resolves
// to a WhatsApp account in a different organisation than the contact's own org.
func (a *App) findOrgMismatchedContacts() ([]OrgMismatchRecord, error) {
	// Query contacts that have a phone_number_id in metadata and whose resolved
	// WhatsApp account org differs from the contact's org.
	type row struct {
		ContactID     string
		PhoneNumber   string
		ProfileName   string
		CurrentOrgID  string
		PhoneNumberID string
	}

	var rows []row
	if err := a.DB.Raw(`
		SELECT
			c.id          AS contact_id,
			c.phone_number AS phone_number,
			c.profile_name AS profile_name,
			c.organization_id::text AS current_org_id,
			c.metadata->>'phone_number_id' AS phone_number_id
		FROM contacts c
		WHERE
			c.deleted_at IS NULL
			AND c.metadata->>'phone_number_id' IS NOT NULL
			AND c.metadata->>'phone_number_id' != ''
	`).Scan(&rows).Error; err != nil {
		return nil, err
	}

	// Build map of phone_number_id -> whatsapp_accounts
	type waRow struct {
		PhoneID        string
		OrganizationID string
		Name           string
	}
	var waRows []waRow
	if err := a.DB.Raw(`
		SELECT phone_id, organization_id::text, name
		FROM whatsapp_accounts
		WHERE deleted_at IS NULL
	`).Scan(&waRows).Error; err != nil {
		return nil, err
	}
	phoneIDToAccount := make(map[string]waRow)
	for _, wa := range waRows {
		phoneIDToAccount[wa.PhoneID] = wa
	}

	// Build org name cache
	orgNames := make(map[string]string)
	var orgs []models.Organization
	if err := a.DB.Select("id", "name").Find(&orgs).Error; err != nil {
		return nil, err
	}
	for _, o := range orgs {
		orgNames[o.ID.String()] = o.Name
	}

	var results []OrgMismatchRecord
	for _, r := range rows {
		wa, ok := phoneIDToAccount[r.PhoneNumberID]
		if !ok {
			// phone_number_id doesn't match any account — skip
			continue
		}
		if wa.OrganizationID == r.CurrentOrgID {
			// Already in the right org
			continue
		}

		// Count messages for this contact
		var msgCount int64
		a.DB.Model(&models.Message{}).Where("contact_id = ?", r.ContactID).Count(&msgCount)

		results = append(results, OrgMismatchRecord{
			ContactID:      r.ContactID,
			PhoneNumber:    r.PhoneNumber,
			ProfileName:    r.ProfileName,
			CurrentOrgID:   r.CurrentOrgID,
			CurrentOrgName: orgNames[r.CurrentOrgID],
			CorrectOrgID:   wa.OrganizationID,
			CorrectOrgName: orgNames[wa.OrganizationID],
			PhoneNumberID:  r.PhoneNumberID,
			MessageCount:   int(msgCount),
		})
	}

	return results, nil
}
