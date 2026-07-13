package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/nikyjain/whatomate/internal/models"
)

// starterSteps mirrors the RSVP starter flow: member attendance -> spouse
// attendance -> spouse mobile (only if spouse attending).
func starterSteps() []models.ChatbotFlowStep {
	att := models.JSONBArray{
		map[string]interface{}{"id": "yes", "title": "Attending"},
		map[string]interface{}{"id": "no", "title": "Not Attending"},
	}
	return []models.ChatbotFlowStep{
		{StepName: "attendance", StepOrder: 1, Buttons: att, StoreAs: "attendance",
			ConditionalNext: models.JSONB{"yes": "spouse_attendance", "no": "spouse_attendance", "default": ""}},
		{StepName: "spouse_attendance", StepOrder: 2, Buttons: att, StoreAs: "spouse_attendance",
			ConditionalNext: models.JSONB{"yes": "spouse_mobile", "no": "__complete__", "default": ""}},
		{StepName: "spouse_mobile", StepOrder: 3, InputType: models.InputType("phone"),
			StoreAs: "spouse_mobile", NextStep: "__complete__"},
	}
}

func TestReplayRSVPAnswers(t *testing.T) {
	t.Parallel()
	steps := starterSteps()
	ev := "11111111-1111-1111-1111-111111111111"

	// Full journey.
	got := replayRSVPAnswers(steps, ev, []string{"Attending", "Attending", "9876543210"})
	assert.Equal(t, "yes", got["attendance"])
	assert.Equal(t, "Attending", got["attendance_title"])
	assert.Equal(t, "yes", got["spouse_attendance"])
	assert.Equal(t, "Attending", got["spouse_attendance_title"])
	assert.Equal(t, "9876543210", got["spouse_mobile"])

	// Member only (dropped off before spouse question).
	got = replayRSVPAnswers(steps, ev, []string{"Attending"})
	assert.Equal(t, "yes", got["attendance"])
	assert.NotContains(t, got, "spouse_attendance")
	assert.NotContains(t, got, "spouse_mobile")

	// Spouse Not Attending → no mobile expected, and it terminates cleanly.
	got = replayRSVPAnswers(steps, ev, []string{"Not Attending", "Not Attending"})
	assert.Equal(t, "no", got["attendance"])
	assert.Equal(t, "no", got["spouse_attendance"])
	assert.NotContains(t, got, "spouse_mobile")

	// Leading trigger-keyword noise is skipped, replies still align.
	got = replayRSVPAnswers(steps, ev, []string{"RSVP", "Attending", "Not Attending"})
	assert.Equal(t, "yes", got["attendance"])
	assert.Equal(t, "no", got["spouse_attendance"])

	// No usable replies → nil.
	assert.Nil(t, replayRSVPAnswers(steps, ev, []string{"hello", "random"}))
	assert.Nil(t, replayRSVPAnswers(steps, ev, []string{}))
}
