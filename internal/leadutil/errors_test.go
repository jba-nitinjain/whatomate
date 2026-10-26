package leadutil

import (
	"errors"
	"fmt"
	"testing"

	"github.com/nikyjain/whatomate/pkg/whatsapp"
)

func TestClassify(t *testing.T) {
	tokenErr := whatsapp.ParseMetaAPIError(400, []byte(`{"error":{"message":"bad token","code":190}}`))
	rateErr := whatsapp.ParseMetaAPIError(400, []byte(`{"error":{"message":"slow down","code":4}}`))
	serverErr := whatsapp.ParseMetaAPIError(503, []byte(`unavailable`))

	cases := []struct {
		name      string
		err       error
		retryable bool
		code      int
	}{
		{"typed token error", tokenErr, false, CodeTokenInvalid},
		{"wrapped rate limit", fmt.Errorf("fetch: %w", rateErr), true, 4},
		{"http 5xx", serverErr, true, 0},
		{"persisted text opt-out", errors.New("API error 131050: user stopped marketing"), false, CodeMarketingOptOut},
		{"network timeout", errors.New("request failed: context deadline exceeded"), true, 0},
		// Plain numbers in free text must not be mistaken for Meta codes.
		{"unrelated digits", errors.New("phone 9190500123 rejected"), false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			retryable, reason := Classify(tc.err)
			if retryable != tc.retryable {
				t.Fatalf("retryable=%v want %v (reason %q)", retryable, tc.retryable, reason)
			}
			code, _, _ := MetaErrorCode(tc.err)
			if code != tc.code {
				t.Fatalf("code=%d want %d", code, tc.code)
			}
		})
	}
}
