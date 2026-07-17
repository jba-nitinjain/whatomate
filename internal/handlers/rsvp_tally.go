package handlers

import (
	"strings"

	"github.com/nikyjain/whatomate/internal/models"
)

type rsvpAttendanceCounts struct {
	Attending    int `json:"attending"`
	NotAttending int `json:"not_attending"`
	Maybe        int `json:"maybe"`
	Pending      int `json:"pending"`
}

type rsvpAttendanceBreakdown struct {
	Member rsvpAttendanceCounts `json:"member_attendance"`
	Spouse rsvpAttendanceCounts `json:"spouse_attendance"`
}

func normalizedRSVPAnswer(answers models.JSONB, keys ...string) string {
	for _, key := range keys {
		value, ok := answers[key].(string)
		if ok && strings.TrimSpace(value) != "" {
			return strings.ToLower(strings.TrimSpace(value))
		}
	}
	return ""
}

func addAttendanceCount(counts *rsvpAttendanceCounts, value string) {
	switch value {
	case "yes", "attending":
		counts.Attending++
	case "no", "not attending", "not_attending":
		counts.NotAttending++
	case "maybe":
		counts.Maybe++
	default:
		counts.Pending++
	}
}

func buildRSVPAttendanceBreakdown(responses []models.RSVPResponse, spouseMobileField string) rsvpAttendanceBreakdown {
	if strings.TrimSpace(spouseMobileField) == "" {
		spouseMobileField = "spouse_mobile"
	}
	var result rsvpAttendanceBreakdown
	for _, response := range responses {
		addAttendanceCount(&result.Member, string(response.Attendance))

		spouseAnswer := normalizedRSVPAnswer(response.Answers, "spouse_attendance", "spouse_attendance_title")
		if spouseAnswer == "yes" || spouseAnswer == "attending" {
			result.Spouse.Attending++
			mobile := normalizedRSVPAnswer(response.Answers, spouseMobileField)
			if len(normalizePhoneDigits(mobile)) < 10 {
				// Attendance and completion are independent: keep the spouse in
				// Attending while also flagging the incomplete contact as Pending.
				result.Spouse.Pending++
			}
			continue
		}
		addAttendanceCount(&result.Spouse, spouseAnswer)
	}
	return result
}

// rsvpContributorTally is one configured contributor's totals across all responses.
type rsvpContributorTally struct {
	Label       string `json:"label"`
	AnswerKey   string `json:"answer_key"`
	Mode        string `json:"mode"`
	People      int    `json:"people"`
	Responses   int    `json:"responses"`
	NeedsReview int    `json:"needs_review"`
	Unparseable int    `json:"unparseable"`
}

// buildRSVPHeadcount tallies every configured contributor and sums the grand total.
// This replaces the hardcoded spouse_attendance logic: children, drivers or anything
// else are rows in the event's configuration, not branches in this function.
func buildRSVPHeadcount(responses []models.RSVPResponse, contributors models.RSVPHeadcountContributors) ([]rsvpContributorTally, int) {
	tallies := make([]rsvpContributorTally, 0, len(contributors))
	total := 0

	for _, contributor := range contributors {
		tally := rsvpContributorTally{
			Label:     contributor.Label,
			AnswerKey: contributor.AnswerKey,
			Mode:      string(contributor.Mode),
		}
		for _, response := range responses {
			got := evaluateHeadcountContributor(contributor, response.Answers, response.Attendance)
			tally.People += got.People
			if got.Matched {
				tally.Responses++
			}
			if got.NeedsReview {
				tally.NeedsReview++
			}
			if got.Unparseable {
				tally.Unparseable++
			}
		}
		total += tally.People
		tallies = append(tallies, tally)
	}

	return tallies, total
}
