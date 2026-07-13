package handlers

import (
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"

	"github.com/nikyjain/whatomate/internal/models"
)

// sessionHasAnswers reports whether a data map holds at least one real answer
// (any key not prefixed with '_'), so we don't stamp empty guests as responded.
func sessionHasAnswers(data models.JSONB) bool {
	for k := range data {
		if len(k) > 0 && k[0] != '_' {
			return true
		}
	}
	return false
}

type flowButton struct{ id, title string }

func stepButtons(step *models.ChatbotFlowStep) []flowButton {
	out := []flowButton{}
	for _, raw := range step.Buttons {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		title, _ := m["title"].(string)
		out = append(out, flowButton{id: id, title: title})
	}
	return out
}

// nextStepName resolves the next step name after answering `step` with `key`
// (button id or matched title): conditional_next[key] -> conditional_next[default]
// -> next_step.
func nextStepName(step *models.ChatbotFlowStep, key string) string {
	if step.ConditionalNext != nil {
		if v, ok := step.ConditionalNext[key].(string); ok && v != "" {
			return v
		}
		if v, ok := step.ConditionalNext["default"].(string); ok && v != "" {
			return v
		}
	}
	return step.NextStep
}

// replayRSVPAnswers walks the flow graph over the ordered visible texts of a
// guest's inbound replies, rebuilding the answers map (button id + _title, or raw
// text) exactly as the live engine would have stored it. Pure — no DB.
func replayRSVPAnswers(steps []models.ChatbotFlowStep, eventID string, contents []string) models.JSONB {
	if len(steps) == 0 {
		return nil
	}
	byName := map[string]*models.ChatbotFlowStep{}
	for i := range steps {
		byName[steps[i].StepName] = &steps[i]
	}

	data := models.JSONB{rsvpEventIDKey: eventID}
	idx := 0
	step := &steps[0]
	for guard := 0; step != nil && guard < 64; guard++ {
		buttons := stepButtons(step)
		if len(buttons) > 0 {
			// Scan forward for the first reply matching a button title.
			matched := -1
			var mb flowButton
			for j := idx; j < len(contents); j++ {
				txt := strings.TrimSpace(contents[j])
				for _, b := range buttons {
					if strings.EqualFold(txt, strings.TrimSpace(b.title)) {
						matched, mb = j, b
						break
					}
				}
				if matched >= 0 {
					break
				}
			}
			if matched < 0 {
				break // guest never answered this step
			}
			if step.StoreAs != "" {
				data[step.StoreAs] = mb.id
				data[step.StoreAs+"_title"] = mb.title
			}
			idx = matched + 1
			step = byName[nextStepName(step, mb.id)]
		} else if step.InputType != models.InputTypeNone && step.InputType != "" {
			// Free-text step (e.g. spouse mobile): take the next reply.
			if idx >= len(contents) {
				break
			}
			val := strings.TrimSpace(contents[idx])
			if step.StoreAs != "" {
				data[step.StoreAs] = val
			}
			idx++
			step = byName[nextStepName(step, val)]
		} else {
			// Message-only step: advance without consuming input.
			step = byName[step.NextStep]
		}
	}

	if !sessionHasAnswers(data) {
		return nil
	}
	return data
}

// reconstructRSVPFromChat replays the flow over a contact's stored inbound
// messages (which keep the visible reply text) to rebuild the answers a guest
// gave before dropping off — no reliance on the live session, which may be gone.
func (a *App) reconstructRSVPFromChat(orgID uuid.UUID, event *models.RSVPEvent, steps []models.ChatbotFlowStep, contactID uuid.UUID) models.JSONB {
	var msgs []models.Message
	a.DB.Where("organization_id = ? AND contact_id = ? AND direction = ?",
		orgID, contactID, models.DirectionIncoming).
		Order("created_at ASC").Find(&msgs)
	if len(msgs) == 0 {
		return nil
	}
	contents := make([]string, len(msgs))
	for i := range msgs {
		contents[i] = msgs[i].Content
	}
	return replayRSVPAnswers(steps, event.ID.String(), contents)
}

// RecoverRSVPPartials commits partial answers for guests who dropped off mid-flow.
// It first finalizes any live chatbot sessions still holding partial data, then
// reconstructs answers from the stored chat history for every still-incomplete
// guest — so recovery works even when the session is long gone. Idempotent.
func (a *App) RecoverRSVPPartials(r *fastglue.Request) error {
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
	if event.FlowID == nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "No flow is linked to this event", nil, "")
	}
	flow, err := a.getChatbotFlowByIDCached(orgID, *event.FlowID)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to load flow", nil, "")
	}

	recovered := map[uuid.UUID]bool{}

	// Pass 1: finalize live sessions still carrying partial data.
	var sessions []models.ChatbotSession
	if err := a.DB.
		Where("organization_id = ? AND session_data->>? = ?", orgID, rsvpEventIDKey, eventID.String()).
		Order("last_activity_at ASC").Find(&sessions).Error; err == nil {
		for i := range sessions {
			if !sessionHasAnswers(sessions[i].SessionData) {
				continue
			}
			a.finalizeRSVPFromSession(&sessions[i])
			recovered[sessions[i].ContactID] = true
		}
	}

	// Pass 2: reconstruct from chat history for every still-incomplete guest.
	steps := append([]models.ChatbotFlowStep(nil), flow.Steps...)
	sort.SliceStable(steps, func(i, j int) bool { return steps[i].StepOrder < steps[j].StepOrder })

	targets, err := a.computeRepromptTargets(orgID, event)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to compute targets", nil, "")
	}
	for _, tgt := range targets {
		data := a.reconstructRSVPFromChat(orgID, event, steps, tgt.ContactID)
		if data == nil {
			continue
		}
		a.finalizeRSVPFromSession(&models.ChatbotSession{
			OrganizationID: orgID,
			ContactID:      tgt.ContactID,
			PhoneNumber:    tgt.Phone,
			SessionData:    data,
		})
		recovered[tgt.ContactID] = true
	}

	return r.SendEnvelope(map[string]interface{}{"recovered": len(recovered)})
}
