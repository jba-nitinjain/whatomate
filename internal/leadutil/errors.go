package leadutil

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/nikyjain/whatomate/pkg/whatsapp"
)

// Meta error codes the lead pipeline reacts to.
const (
	CodeTokenInvalid     = 190
	CodeMarketingOptOut  = 131050
	codePermission       = 200
	codePermissionLegacy = 10
	codeNotFound         = 100
)

// metaCodeRe extracts the Meta error code from the message produced by
// whatsapp.ParseMetaAPIError ("API error <code>: ..."). It is only used when
// the typed error is no longer available, e.g. after a send failure was
// persisted as text on the message row.
var metaCodeRe = regexp.MustCompile(`API error (\d+):`)

// MetaErrorCode returns the Meta error code and HTTP status carried by err.
// It prefers the typed *whatsapp.GraphError and falls back to parsing the
// canonical "API error <code>:" prefix. ok is false when no code is present.
func MetaErrorCode(err error) (code, httpStatus int, ok bool) {
	if err == nil {
		return 0, 0, false
	}
	var ge *whatsapp.GraphError
	if errors.As(err, &ge) {
		return ge.Code, ge.StatusCode, ge.Code != 0 || ge.StatusCode != 0
	}
	if m := metaCodeRe.FindStringSubmatch(err.Error()); m != nil {
		if c, convErr := strconv.Atoi(m[1]); convErr == nil {
			return c, 0, true
		}
	}
	return 0, 0, false
}

// Classify classifies errors from Meta Graph API / WhatsApp Cloud API into
// retryable vs permanent failures and returns a human readable reason.
func Classify(err error) (retryable bool, reason string) {
	if err == nil {
		return false, ""
	}
	errMsg := err.Error()

	code, status, hasCode := MetaErrorCode(err)
	if hasCode && code != 0 {
		if r, reason, known := classifyMetaCode(code, errMsg); known {
			return r, reason
		}
	}
	if hasCode && status >= 500 {
		return true, errMsg
	}
	if hasCode && status == 429 {
		return true, errMsg
	}

	lower := strings.ToLower(errMsg)
	if strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "deadline exceeded") ||
		strings.Contains(lower, "request failed") {
		return true, errMsg
	}

	// Unknown errors are treated as permanent so a bad lead cannot loop forever.
	return false, errMsg
}

func classifyMetaCode(code int, errMsg string) (retryable bool, reason string, known bool) {
	switch code {
	case CodeTokenInvalid:
		return false, fmt.Sprintf("Graph API token expired or invalid (code %d)", code), true
	case codePermission, codePermissionLegacy:
		return false, fmt.Sprintf("Missing leads_retrieval permission or Leads Access Manager restriction (code %d)", code), true
	case codeNotFound:
		return false, fmt.Sprintf("Lead or object not found (code %d)", code), true
	case 4, 17, 32, 613:
		return true, fmt.Sprintf("Graph API rate limit (code %d)", code), true
	case 131056, 130429:
		return true, fmt.Sprintf("WhatsApp rate/throughput limit (code %d)", code), true
	case 131026:
		return false, fmt.Sprintf("Recipient not on WhatsApp (code %d)", code), true
	case 132000, 132001, 132018:
		return false, fmt.Sprintf("Template parameters or name invalid: %s", errMsg), true
	case 131049:
		return false, fmt.Sprintf("Marketing frequency cap reached (code %d)", code), true
	case CodeMarketingOptOut:
		return false, fmt.Sprintf("User opted out of marketing (code %d)", code), true
	}
	return false, "", false
}
