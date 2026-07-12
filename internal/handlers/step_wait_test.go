package handlers

import (
	"testing"

	"github.com/nikyjain/whatomate/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestStepHasReplyButtons(t *testing.T) {
	t.Parallel()

	reply := &models.ChatbotFlowStep{
		MessageType: models.FlowStepTypeButtons,
		Buttons:     models.JSONBArray{map[string]interface{}{"id": "yes", "title": "Attending"}},
	}
	assert.True(t, stepHasReplyButtons(reply), "reply buttons (no type) must wait")

	explicitReply := &models.ChatbotFlowStep{
		MessageType: models.FlowStepTypeButtons,
		Buttons:     models.JSONBArray{map[string]interface{}{"id": "a", "title": "A", "type": "reply"}},
	}
	assert.True(t, stepHasReplyButtons(explicitReply))

	ctaOnly := &models.ChatbotFlowStep{
		MessageType: models.FlowStepTypeButtons,
		Buttons:     models.JSONBArray{map[string]interface{}{"title": "Open", "type": "url", "url": "https://x"}},
	}
	assert.False(t, stepHasReplyButtons(ctaOnly), "URL/CTA buttons do not wait for a tap")

	textStep := &models.ChatbotFlowStep{MessageType: models.FlowStepTypeText}
	assert.False(t, stepHasReplyButtons(textStep))

	noButtons := &models.ChatbotFlowStep{MessageType: models.FlowStepTypeButtons}
	assert.False(t, stepHasReplyButtons(noButtons))
}
