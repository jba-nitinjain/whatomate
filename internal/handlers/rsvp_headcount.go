package handlers

import (
	"regexp"
	"strconv"
	"strings"
)

// headcountReviewCeiling is the largest count accepted without being flagged for a
// human look. Above it the value is still counted - it is flagged, not rejected.
const headcountReviewCeiling = 20

// headcountDigitsPattern captures an optional leading sign so a negative answer is
// recognised as nonsense rather than silently becoming a positive count.
var headcountDigitsPattern = regexp.MustCompile(`-?\d+`)

// headcountWords maps the number words a guest might type. Kept deliberately small:
// beyond ten, a typed word is more likely a typo than a real count.
var headcountWords = map[string]int{
	"zero": 0, "one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
	"six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
}

// headcountNoneWords are answers that explicitly mean zero.
var headcountNoneWords = map[string]struct{}{
	"no": {}, "none": {}, "nil": {}, "nope": {}, "n/a": {}, "na": {},
}

// parseHeadcountValue reads a guest's free-text count leniently. ok=false means the
// answer could not be understood; the caller counts 0 and flags the row rather than
// silently losing a family.
func parseHeadcountValue(raw string) (int, bool) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return 0, true
	}
	if _, none := headcountNoneWords[s]; none {
		return 0, true
	}
	if match := headcountDigitsPattern.FindString(s); match != "" {
		n, err := strconv.Atoi(match)
		if err != nil {
			return 0, false
		}
		if n < 0 {
			// A negative count is nonsense. Flag it for a human rather than
			// silently inventing a positive count from it.
			return 0, false
		}
		return n, true
	}
	for word, n := range headcountWords {
		if strings.Contains(s, word) {
			return n, true
		}
	}
	return 0, false
}

// headcountNeedsReview reports whether a parsed count is implausible enough to show
// to a human. The value still counts.
func headcountNeedsReview(value int) bool {
	return value > headcountReviewCeiling
}
