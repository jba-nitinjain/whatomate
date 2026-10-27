package handlers

import (
	"strconv"
	"strings"
)

// urlButtonParamSource returns the parameter map used to resolve dynamic URL
// button values. Explicit button_params entries (keyed by button index) are
// exposed as "button_url_<index>", which templateutil resolves before
// template_params, so a caller-supplied button_params value both fills the
// URL button and satisfies the missing-parameter check. Without button_params
// this is exactly template_params (main behaviour).
func urlButtonParamSource(templateParams, buttonParams map[string]string) map[string]string {
	if len(buttonParams) == 0 {
		return templateParams
	}
	merged := make(map[string]string, len(templateParams)+len(buttonParams))
	for k, v := range templateParams {
		merged[k] = v
	}
	for k, v := range buttonParams {
		if strings.TrimSpace(v) == "" {
			continue
		}
		if _, err := strconv.Atoi(k); err == nil {
			merged["button_url_"+k] = v
		}
	}
	return merged
}

// nonURLButtonParams keeps only button_params entries that target non-URL
// buttons (COPY_CODE / OTP). URL buttons are already resolved into the
// index-keyed ButtonParams map, so they are excluded here.
func nonURLButtonParams(buttons []any, params map[string]string) map[string]string {
	if len(params) == 0 {
		return nil
	}
	out := make(map[string]string, len(params))
	for k, v := range params {
		idx, err := strconv.Atoi(k)
		if err != nil || v == "" {
			continue
		}
		if idx >= 0 && idx < len(buttons) {
			if btn, ok := buttons[idx].(map[string]any); ok {
				if t, _ := btn["type"].(string); strings.EqualFold(t, "URL") {
					continue
				}
			}
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// buttonURLParamsExcluding drops string-indexed button params whose index is
// already present in the int-indexed ButtonParams map.
func buttonURLParamsExcluding(params map[string]string, covered map[int]string) map[string]string {
	if len(params) == 0 {
		return nil
	}
	if len(covered) == 0 {
		return params
	}
	out := make(map[string]string, len(params))
	for k, v := range params {
		if _, dup := covered[mustAtoi(k)]; dup {
			continue
		}
		out[k] = v
	}
	return out
}

func mustAtoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	return n
}
