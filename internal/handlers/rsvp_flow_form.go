package handlers

import (
	"strings"

	"github.com/nikyjain/whatomate/internal/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// buildRSVPFormScreen builds a single-screen WhatsApp Flow form that collects a
// mobile number, attendance, and an optional spouse mobile. The completion payload
// keys equal the RSVP event's answer keys so the submission maps straight into the
// RSVP response (attendance is derived from attendanceField).
func buildRSVPFormScreen(attendanceField, spouseField string) map[string]interface{} {
	return map[string]interface{}{
		"id":       "RSVP",
		"title":    "RSVP",
		"terminal": true,
		"layout": map[string]interface{}{
			"type": "SingleColumnLayout",
			"children": []interface{}{
				map[string]interface{}{"type": "TextHeading", "text": "Please confirm your RSVP"},
				map[string]interface{}{
					"type": "TextInput", "name": "mobile", "label": "Mobile Number",
					"input-type": "phone", "required": true,
				},
				map[string]interface{}{
					"type": "RadioButtonsGroup", "name": attendanceField, "label": "Will you attend?",
					"required": true,
					"data-source": []interface{}{
						map[string]interface{}{"id": "yes", "title": "Attending"},
						map[string]interface{}{"id": "no", "title": "Not Attending"},
					},
				},
				map[string]interface{}{
					"type": "TextInput", "name": spouseField, "label": "Spouse mobile (optional)",
					"input-type": "phone",
				},
				map[string]interface{}{
					"type": "Footer", "label": "Submit",
					"on-click-action": map[string]interface{}{
						"name": "complete",
						"payload": map[string]interface{}{
							"mobile":        "${form.mobile}",
							attendanceField: "${form." + attendanceField + "}",
							spouseField:     "${form." + spouseField + "}",
						},
					},
				},
			},
		},
	}
}

// GenerateRSVPFlowForm creates a DRAFT WhatsApp Flow form (mobile + attendance +
// spouse mobile) for an RSVP event. The user reviews it in Flows, then Saves to
// Meta and Publishes, and finally sends it from a whatsapp_flow chatbot step.
func (a *App) GenerateRSVPFlowForm(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	eventID, err := parsePathUUID(r, "id", "RSVP event")
	if err != nil {
		return nil
	}
	event, err := findByIDAndOrg[models.RSVPEvent](a.DB, r, eventID, orgID, "RSVP event")
	if err != nil {
		return nil
	}
	if strings.TrimSpace(event.WhatsAppAccount) == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Set a WhatsApp account on the event first", nil, "")
	}

	attendanceField := firstNonEmpty(strings.TrimSpace(event.AttendanceField), "attendance")
	spouseField := firstNonEmpty(strings.TrimSpace(event.SpouseMobileField), "spouse_mobile")

	name := "RSVP form – " + event.Name
	if len(name) > 255 {
		name = name[:255]
	}

	flow := models.WhatsAppFlow{
		OrganizationID:  orgID,
		WhatsAppAccount: event.WhatsAppAccount,
		Name:            name,
		Status:          "DRAFT",
		Category:        "OTHER",
		JSONVersion:     "6.0",
		Screens:         models.JSONBArray{buildRSVPFormScreen(attendanceField, spouseField)},
		HasLocalChanges: true,
	}
	if err := a.DB.Create(&flow).Error; err != nil {
		a.Log.Error("Failed to create RSVP flow form", "error", err, "event_id", eventID)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to create RSVP form", nil, "")
	}

	return r.SendEnvelope(map[string]interface{}{
		"flow":    flowToResponse(flow),
		"message": "RSVP form draft created. Open Flows to review, save to Meta, and publish.",
	})
}
