package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Field lists for the phone-number node. The "legacy" lists are exactly what
// our main branch requested; the extended lists add fields newer Graph API
// versions expose (Coexistence detection). Some API versions/apps reject the
// newer fields with "(#100) Tried accessing nonexisting field", so callers
// retry once with the legacy list rather than failing.
const (
	PhoneStatusFieldsLegacy   = "display_phone_number,verified_name,code_verification_status,account_mode,quality_rating,messaging_limit_tier"
	PhoneStatusFields         = PhoneStatusFieldsLegacy + ",is_on_biz_app,platform_type"
	PhoneValidateFieldsLegacy = "display_phone_number,verified_name,code_verification_status,account_mode,quality_rating"
	PhoneValidateFields       = PhoneValidateFieldsLegacy + ",is_on_biz_app,platform_type"
	PhoneInfoFieldsLegacy     = "verified_name,display_phone_number,quality_rating"
	PhoneInfoFields           = PhoneInfoFieldsLegacy + ",is_on_biz_app,platform_type"
)

// IsUnknownFieldError reports whether err is a Graph API error rejecting a
// requested field (code 100, e.g. "Tried accessing nonexisting field").
func IsUnknownFieldError(err error) bool {
	if err == nil {
		return false
	}
	var ge *GraphError
	if errors.As(err, &ge) {
		return isUnknownFieldMessage(ge.Code, ge.Error())
	}
	return isUnknownFieldMessage(0, err.Error())
}

// IsUnknownFieldResponse is IsUnknownFieldError for a raw HTTP response.
func IsUnknownFieldResponse(statusCode int, body []byte) bool {
	if statusCode == http.StatusOK {
		return false
	}
	return IsUnknownFieldError(ParseMetaAPIError(statusCode, body))
}

func isUnknownFieldMessage(code int, msg string) bool {
	m := strings.ToLower(msg)
	if strings.Contains(m, "nonexisting field") || strings.Contains(m, "unknown field") || strings.Contains(m, "invalid field") {
		return true
	}
	return (code == 100 || strings.Contains(m, "(#100)")) && strings.Contains(m, "field")
}

// getNodeFields GETs a Graph node with the extended field list and, if Meta
// rejects one of the fields, retries once with the legacy list.
func (c *Client) getNodeFields(ctx context.Context, apiVersion, nodeID, fields, legacyFields, accessToken string) ([]byte, error) {
	url := fmt.Sprintf("%s/%s/%s?fields=%s", c.getBaseURL(), apiVersion, nodeID, fields)
	body, err := c.doRequest(ctx, http.MethodGet, url, nil, accessToken)
	if err == nil || legacyFields == "" || legacyFields == fields || !IsUnknownFieldError(err) {
		return body, err
	}
	c.Log.Warn("Meta rejected extended phone fields, retrying with legacy field list", "node_id", nodeID, "error", err)
	url = fmt.Sprintf("%s/%s/%s?fields=%s", c.getBaseURL(), apiVersion, nodeID, legacyFields)
	return c.doRequest(ctx, http.MethodGet, url, nil, accessToken)
}
