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

// headcountWord pairs a recognised number word with its value.
type headcountWord struct {
	word string
	n    int
}

// headcountWords lists the number words a guest might type, in a fixed order.
// This is a slice, not a map, on purpose: Go map iteration order is randomized,
// and the old map-based scan returned on the first hit it happened to visit -
// so an answer containing two number words parsed to a different count on
// different runs. Kept deliberately small: beyond ten, a typed word is more
// likely a typo than a real count.
var headcountWords = []headcountWord{
	{"zero", 0}, {"one", 1}, {"two", 2}, {"three", 3}, {"four", 4}, {"five", 5},
	{"six", 6}, {"seven", 7}, {"eight", 8}, {"nine", 9}, {"ten", 10},
}

// headcountWordPattern matches any word from headcountWords as a whole word only,
// so "none at all" does not accidentally match "one" as a substring of "none".
var headcountWordPattern = newHeadcountWordPattern(headcountWords)

func newHeadcountWordPattern(words []headcountWord) *regexp.Regexp {
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = w.word
	}
	return regexp.MustCompile(`\b(` + strings.Join(parts, "|") + `)\b`)
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

	// Collect every numeric reading from both the digit matches and the word
	// matches into one set, then judge ambiguity on the unified set rather
	// than on digits and words separately. Otherwise a digit reading returns
	// unconditionally without ever consulting the word matches, so an answer
	// like "2 or three" or "one, actually 2" would silently discard the
	// conflicting word/digit and report only the digit. Repeats of the same
	// value, or a digit and a word that agree (e.g. "1 or one"), collapse to
	// one reading below and are not ambiguous.
	readings := map[int]struct{}{}

	if matches := headcountDigitsPattern.FindAllString(s, -1); len(matches) > 0 {
		for _, match := range matches {
			v, err := strconv.Atoi(match)
			if err != nil {
				return 0, false
			}
			readings[v] = struct{}{}
		}
	}

	if matches := headcountWordPattern.FindAllString(s, -1); len(matches) > 0 {
		for _, match := range matches {
			for _, w := range headcountWords {
				if w.word == match {
					readings[w.n] = struct{}{}
					break
				}
			}
		}
	}

	if len(readings) == 1 {
		for n := range readings {
			if n < 0 {
				// A negative count is nonsense. Flag it for a human rather
				// than silently inventing a positive count from it.
				return 0, false
			}
			return n, true
		}
	}
	// Zero readings: nothing recognisable as a number, digit or word.
	// More than one distinct reading ("1 or 2", "1 2", "2 or three", "one,
	// actually 2") is ambiguous - a human decides, we don't silently pick one
	// and discard the rest. Note "2-3" lands here too, but not because it's a
	// range: headcountDigitsPattern is `-?\d+`, so FindAllString("2-3")
	// yields ["2", "-3"] - the hyphen is captured as a minus sign, not
	// recognised as a range separator - giving readings {2, -3}, not {2, 3}.
	return 0, false
}

// headcountNeedsReview reports whether a parsed count is implausible enough to show
// to a human. The value still counts.
func headcountNeedsReview(value int) bool {
	return value > headcountReviewCeiling
}
