package templateutil

import (
	"fmt"
	"regexp"
	"strings"
)

// ParameterPattern matches template parameters like {{1}}, {{name}}, {{order_id}}
var ParameterPattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// ExtParamNames extracts parameter names from template content.
// Supports both positional ({{1}}, {{2}}) and named ({{name}}, {{order_id}}) parameters.
// Returns parameter names in order of first occurrence, without duplicates.
func ExtParamNames(content string) []string {
	matches := ParameterPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var names []string
	for _, match := range matches {
		if len(match) > 1 {
			name := strings.TrimSpace(match[1])
			if name != "" && !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}

// ResolveParamsFromMap resolves both positional and named parameters to ordered values
// using a map[string]string parameter source.
func ResolveParamsFromMap(paramNames []string, params map[string]string) []string {
	if len(paramNames) == 0 || len(params) == 0 {
		return nil
	}

	result := make([]string, len(paramNames))
	for i, name := range paramNames {
		// Try named key first
		if val, ok := params[name]; ok {
			result[i] = val
			continue
		}
		// Fall back to positional key (1-indexed)
		key := fmt.Sprintf("%d", i+1)
		if val, ok := params[key]; ok {
			result[i] = val
			continue
		}
		// Default to empty string
		result[i] = ""
	}
	return result
}

// ResolveParamsMapFromMap resolves parameters into a name->value map using template parameter names.
// It supports both named keys (e.g. "name") and positional keys (e.g. "1").
func ResolveParamsMapFromMap(paramNames []string, params map[string]string) map[string]string {
	if len(paramNames) == 0 || len(params) == 0 {
		return nil
	}

	result := make(map[string]string, len(paramNames))
	for i, name := range paramNames {
		if val, ok := params[name]; ok {
			result[name] = val
			continue
		}
		key := fmt.Sprintf("%d", i+1)
		if val, ok := params[key]; ok {
			result[name] = val
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// ResolveParams resolves both positional and named parameters to ordered values
// using a map[string]interface{} parameter source (e.g. models.JSONB).
func ResolveParams(bodyContent string, params map[string]interface{}) []string {
	if len(params) == 0 {
		return nil
	}

	paramNames := ExtParamNames(bodyContent)
	if len(paramNames) == 0 {
		return nil
	}

	result := make([]string, len(paramNames))
	for i, name := range paramNames {
		// Try named key first
		if val, ok := params[name]; ok {
			result[i] = fmt.Sprintf("%v", val)
			continue
		}
		// Fall back to positional key (1-indexed)
		key := fmt.Sprintf("%d", i+1)
		if val, ok := params[key]; ok {
			result[i] = fmt.Sprintf("%v", val)
			continue
		}
		// Default to empty string
		result[i] = ""
	}
	return result
}

// ResolveParamsMap resolves parameters into a name->value map using a JSON-like parameter source.
func ResolveParamsMap(content string, params map[string]interface{}) map[string]string {
	paramNames := ExtParamNames(content)
	if len(paramNames) == 0 || len(params) == 0 {
		return nil
	}

	result := make(map[string]string, len(paramNames))
	for i, name := range paramNames {
		if val, ok := params[name]; ok {
			result[name] = fmt.Sprintf("%v", val)
			continue
		}
		key := fmt.Sprintf("%d", i+1)
		if val, ok := params[key]; ok {
			result[name] = fmt.Sprintf("%v", val)
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// ResolveURLButtonParamsFromMap extracts dynamic URL button parameters for template sending.
// It looks up named placeholders directly and also supports explicit button-specific keys like
// button_0 and button_url_0.
func ResolveURLButtonParamsFromMap(buttons []interface{}, params map[string]string) (map[int]string, []string) {
	if len(buttons) == 0 {
		return nil, nil
	}

	resolved := make(map[int]string)
	missingSet := make(map[string]struct{})

	for idx, button := range buttons {
		btnMap, ok := button.(map[string]interface{})
		if !ok {
			continue
		}
		btnType, _ := btnMap["type"].(string)
		if strings.ToUpper(btnType) != "URL" {
			continue
		}

		rawURL, _ := btnMap["url"].(string)
		paramNames := ExtParamNames(rawURL)
		if len(paramNames) == 0 {
			continue
		}

		if val, ok := params[fmt.Sprintf("button_url_%d", idx)]; ok && strings.TrimSpace(val) != "" {
			resolved[idx] = val
			continue
		}
		if val, ok := params[fmt.Sprintf("button_%d", idx)]; ok && strings.TrimSpace(val) != "" {
			resolved[idx] = val
			continue
		}

		found := false
		for _, name := range paramNames {
			if val, ok := params[name]; ok && strings.TrimSpace(val) != "" {
				resolved[idx] = val
				found = true
				break
			}
		}
		if !found {
			missingSet[paramNames[0]] = struct{}{}
		}
	}

	if len(resolved) == 0 {
		resolved = nil
	}

	var missing []string
	for name := range missingSet {
		missing = append(missing, name)
	}

	return resolved, missing
}

// ResolveURLButtonParams extracts dynamic URL button parameters from a JSON-like parameter source.
func ResolveURLButtonParams(buttons []interface{}, params map[string]interface{}) (map[int]string, []string) {
	if len(buttons) == 0 {
		return nil, nil
	}

	resolved := make(map[int]string)
	missingSet := make(map[string]struct{})

	for idx, button := range buttons {
		btnMap, ok := button.(map[string]interface{})
		if !ok {
			continue
		}
		btnType, _ := btnMap["type"].(string)
		if strings.ToUpper(btnType) != "URL" {
			continue
		}

		rawURL, _ := btnMap["url"].(string)
		paramNames := ExtParamNames(rawURL)
		if len(paramNames) == 0 {
			continue
		}

		if val, ok := params[fmt.Sprintf("button_url_%d", idx)]; ok && fmt.Sprintf("%v", val) != "" {
			resolved[idx] = fmt.Sprintf("%v", val)
			continue
		}
		if val, ok := params[fmt.Sprintf("button_%d", idx)]; ok && fmt.Sprintf("%v", val) != "" {
			resolved[idx] = fmt.Sprintf("%v", val)
			continue
		}

		found := false
		for _, name := range paramNames {
			if val, ok := params[name]; ok && fmt.Sprintf("%v", val) != "" {
				resolved[idx] = fmt.Sprintf("%v", val)
				found = true
				break
			}
		}
		if !found {
			missingSet[paramNames[0]] = struct{}{}
		}
	}

	if len(resolved) == 0 {
		resolved = nil
	}

	var missing []string
	for name := range missingSet {
		missing = append(missing, name)
	}

	return resolved, missing
}

// ExtractURLButtonParamNames returns all unique dynamic parameter names referenced by URL buttons.
func ExtractURLButtonParamNames(buttons []interface{}) []string {
	seen := make(map[string]struct{})
	var names []string

	for _, button := range buttons {
		btnMap, ok := button.(map[string]interface{})
		if !ok {
			continue
		}
		btnType, _ := btnMap["type"].(string)
		if strings.ToUpper(btnType) != "URL" {
			continue
		}
		rawURL, _ := btnMap["url"].(string)
		for _, name := range ExtParamNames(rawURL) {
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			names = append(names, name)
		}
	}

	return names
}

// ReplaceWithStringParams replaces {{1}}, {{2}}, {{name}}, etc. placeholders with actual values
// from a map[string]string.
func ReplaceWithStringParams(content string, params map[string]string) string {
	if content == "" || len(params) == 0 {
		return content
	}

	result := content
	paramNames := ExtParamNames(content)
	for i, name := range paramNames {
		// Try to get value by name first (works for both named and positional)
		if val, ok := params[name]; ok {
			result = strings.ReplaceAll(result, fmt.Sprintf("{{%s}}", name), val)
			continue
		}
		// Fall back to positional key (1-indexed)
		key := fmt.Sprintf("%d", i+1)
		if val, ok := params[key]; ok {
			result = strings.ReplaceAll(result, fmt.Sprintf("{{%s}}", name), val)
		}
	}
	return result
}

// ReplaceWithJSONBParams replaces both positional ({{1}}) and named ({{name}}) placeholders
// using a map[string]interface{} parameter source. bodyContent is used to extract parameter
// names (typically the template's body content), and content is the string to perform
// replacements on.
func ReplaceWithJSONBParams(bodyContent, content string, params map[string]interface{}) string {
	if len(params) == 0 {
		return content
	}

	paramNames := ExtParamNames(bodyContent)
	if len(paramNames) == 0 {
		return content
	}

	for i, name := range paramNames {
		// Try named key first
		var val string
		if v, ok := params[name]; ok {
			val = fmt.Sprintf("%v", v)
		} else if v, ok := params[fmt.Sprintf("%d", i+1)]; ok {
			// Fall back to positional key
			val = fmt.Sprintf("%v", v)
		}

		// Replace both named and positional placeholders
		content = strings.ReplaceAll(content, fmt.Sprintf("{{%s}}", name), val)
		content = strings.ReplaceAll(content, fmt.Sprintf("{{%d}}", i+1), val)
	}
	return content
}
