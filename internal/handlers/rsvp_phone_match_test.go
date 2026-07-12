package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPhoneMatchSuffix(t *testing.T) {
	t.Parallel()

	// Differing formats of the same number share the trailing-10-digit suffix.
	cases := []string{"919841274561", "+91 98412 74561", "9841274561", "0091-9841274561"}
	want := "9841274561"
	for _, c := range cases {
		assert.Equal(t, want, phoneMatchSuffix(c), "suffix mismatch for %q", c)
	}

	assert.Equal(t, "", phoneMatchSuffix(""))
	assert.Equal(t, "", phoneMatchSuffix("no-digits"))
	assert.Equal(t, "12345", phoneMatchSuffix("1-2-3-4-5")) // shorter than 10 → whole
	assert.Equal(t, "919841274561", normalizePhoneDigits("+91 98412 74561"))
}
