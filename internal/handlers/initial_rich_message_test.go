package handlers

import (
	"testing"

	"github.com/nikyjain/whatomate/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildInitialRichStep(t *testing.T) {
	t.Parallel()

	// No buttons -> no synthetic step.
	assert.Nil(t, buildInitialRichStep(&models.ChatbotFlow{InitialMessage: "hi"}))

	flow := &models.ChatbotFlow{
		InitialMessage:         "Join our event",
		InitialMediaType:       "image",
		InitialMediaURL:        "https://example.com/poster.jpg",
		InitialButtons:         models.JSONBArray{map[string]interface{}{"id": "yes", "title": "Attending"}},
		InitialConditionalNext: models.JSONB{"yes": "headcount"},
		InitialStoreAs:         "attendance",
	}
	step := buildInitialRichStep(flow)
	require.NotNil(t, step)
	assert.Equal(t, initialRichStepName, step.StepName)
	assert.Equal(t, models.FlowStepTypeButtons, step.MessageType)
	assert.Equal(t, "Join our event", step.Message)
	assert.Equal(t, "attendance", step.StoreAs)
	assert.Equal(t, "image", step.InputConfig["media_type"])
	assert.Equal(t, "https://example.com/poster.jpg", step.InputConfig["media_url"])
	assert.Len(t, step.Buttons, 1)
}

func TestInjectInitialRichStep_Idempotent(t *testing.T) {
	t.Parallel()
	a := &App{}
	flow := &models.ChatbotFlow{
		InitialButtons: models.JSONBArray{map[string]interface{}{"id": "yes", "title": "Yes"}},
		Steps:          []models.ChatbotFlowStep{{StepName: "headcount"}},
	}
	a.injectInitialRichStep(flow)
	a.injectInitialRichStep(flow) // second call must not double-inject
	require.Len(t, flow.Steps, 2)
	assert.Equal(t, initialRichStepName, flow.Steps[0].StepName)
	assert.Equal(t, "headcount", flow.Steps[1].StepName)
}

func TestValidateInitialRichMessage(t *testing.T) {
	t.Parallel()
	steps := []FlowStepRequest{{StepName: "headcount"}}

	assert.NoError(t, validateInitialRichMessage(nil, nil, steps))

	// Too many buttons.
	four := []interface{}{1, 2, 3, 4}
	assert.Error(t, validateInitialRichMessage(four, nil, steps))

	// Unknown target.
	one := []interface{}{map[string]interface{}{"id": "a"}}
	assert.Error(t, validateInitialRichMessage(one, map[string]interface{}{"a": "nope"}, steps))

	// Valid target + terminal sentinel.
	assert.NoError(t, validateInitialRichMessage(one, map[string]interface{}{"a": "headcount"}, steps))
	assert.NoError(t, validateInitialRichMessage(one, map[string]interface{}{"a": "__complete__"}, steps))
}
