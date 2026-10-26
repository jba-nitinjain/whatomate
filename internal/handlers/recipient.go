package handlers

import (
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/pkg/whatsapp"
)

// contactRecipient builds the WhatsApp send target for a contact, carrying the
// Business-Scoped User ID alongside the phone number when Meta has supplied one.
func contactRecipient(contact *models.Contact) whatsapp.Recipient {
	if contact == nil {
		return whatsapp.Recipient{}
	}
	return whatsapp.Recipient{Phone: contact.PhoneNumber, BSUID: contact.BSUID}
}
