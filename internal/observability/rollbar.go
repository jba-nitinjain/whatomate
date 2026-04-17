package observability

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/nikyjain/whatomate/internal/config"
	"github.com/rollbar/rollbar-go"
	"github.com/zerodha/logf"
)

var rollbarEnabled atomic.Bool

// ConfigureRollbar initializes the global Rollbar client if a token is configured.
func ConfigureRollbar(cfg *config.Config, log logf.Logger, codeVersion string) bool {
	token := strings.TrimSpace(cfg.Rollbar.AccessToken)
	if token == "" {
		rollbarEnabled.Store(false)
		return false
	}

	environment := strings.TrimSpace(cfg.Rollbar.Environment)
	if environment == "" {
		environment = strings.TrimSpace(cfg.App.Environment)
	}
	if environment == "" {
		environment = "development"
	}

	rollbar.SetToken(token)
	rollbar.SetEnvironment(environment)
	if codeVersion != "" && codeVersion != "dev" {
		rollbar.SetCodeVersion(codeVersion)
	}
	if endpoint := strings.TrimSpace(cfg.Rollbar.Endpoint); endpoint != "" {
		rollbar.SetEndpoint(endpoint)
	}
	if root, err := os.Getwd(); err == nil {
		rollbar.SetServerRoot(filepath.ToSlash(root))
	}
	rollbar.SetCustom(map[string]interface{}{
		"app":         cfg.App.Name,
		"environment": environment,
		"component":   "whatomate",
	})
	rollbar.SetPrintPayloadOnError(cfg.App.Debug)

	rollbarEnabled.Store(true)
	log.Info("Rollbar enabled", "environment", environment)
	return true
}

// Enabled reports whether Rollbar reporting is active.
func Enabled() bool {
	return rollbarEnabled.Load()
}

// Flush blocks until the current async Rollbar queue is empty.
func Flush() {
	if !Enabled() {
		return
	}
	rollbar.Wait()
}

// Close flushes and closes the async Rollbar client.
func Close() {
	if !Enabled() {
		return
	}
	rollbar.Close()
}

// ReportError reports a non-panic server error to Rollbar.
func ReportError(err error, extras map[string]interface{}) {
	if !Enabled() || err == nil {
		return
	}
	rollbar.ErrorWithStackSkipWithExtras(rollbar.ERR, err, 2, extras)
}

// ReportRecoveredPanic reports a recovered panic to Rollbar.
func ReportRecoveredPanic(recovered interface{}, extras map[string]interface{}) {
	if !Enabled() || recovered == nil {
		return
	}

	var err error
	switch value := recovered.(type) {
	case error:
		err = value
	default:
		err = fmt.Errorf("panic: %v", value)
	}

	rollbar.ErrorWithStackSkipWithExtras(rollbar.CRIT, err, 2, extras)
}
