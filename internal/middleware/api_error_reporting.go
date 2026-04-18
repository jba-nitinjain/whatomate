package middleware

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nikyjain/whatomate/internal/observability"
	"github.com/valyala/fasthttp"
)

type errorEnvelope struct {
	Message string `json:"message"`
}

// ReportAPIErrors wraps the final fasthttp handler and reports any API
// responses with status >= 400 to Rollbar. This catches handled request errors
// that never panic, such as validation and upstream failures.
func ReportAPIErrors(next fasthttp.RequestHandler, basePath string) fasthttp.RequestHandler {
	basePath = strings.TrimSuffix(strings.TrimSpace(basePath), "/")
	apiPrefix := basePath + "/api"
	if apiPrefix == "/api" || apiPrefix == "" {
		apiPrefix = "/api"
	}

	return func(ctx *fasthttp.RequestCtx) {
		next(ctx)

		if !observability.Enabled() {
			return
		}
		if reported, _ := ctx.UserValue(ContextKeyPanicReported).(bool); reported {
			return
		}

		statusCode := ctx.Response.StatusCode()
		if statusCode < fasthttp.StatusBadRequest {
			return
		}

		path := string(ctx.Path())
		if path != apiPrefix && !strings.HasPrefix(path, apiPrefix+"/") {
			return
		}

		message := extractAPIErrorMessage(ctx.Response.Body())
		method := string(ctx.Method())
		errText := fmt.Sprintf("API request failed: %s %s returned %d", method, path, statusCode)
		if message != "" {
			errText += " - " + message
		}

		extras := map[string]interface{}{
			"component":       "http_api_response",
			"method":          method,
			"path":            path,
			"query":           string(ctx.URI().QueryString()),
			"status_code":     statusCode,
			"user_id":         fmt.Sprint(ctx.UserValue(ContextKeyUserID)),
			"organization_id": fmt.Sprint(ctx.UserValue(ContextKeyOrganizationID)),
			"remote_addr":     ctx.RemoteAddr().String(),
		}
		if message != "" {
			extras["response_message"] = message
		}

		observability.ReportError(fmt.Errorf("%s", errText), extras)
	}
}

func extractAPIErrorMessage(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	var envelope errorEnvelope
	if err := json.Unmarshal(body, &envelope); err == nil {
		return truncateErrorMessage(strings.TrimSpace(envelope.Message))
	}

	return truncateErrorMessage(strings.TrimSpace(string(body)))
}

func truncateErrorMessage(message string) string {
	if len(message) <= 500 {
		return message
	}
	return message[:500] + "..."
}
