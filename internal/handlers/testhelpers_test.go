package handlers_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/config"
	"github.com/nikyjain/whatomate/internal/handlers"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/internal/queue"
	"github.com/nikyjain/whatomate/pkg/whatsapp"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/require"
)

// appOption configures an App for testing.
type appOption func(*handlers.App)

// withQueue sets the queue on the test App.
func withQueue(q queue.Queue) appOption {
	return func(a *handlers.App) {
		a.Queue = q
	}
}

// withWhatsApp sets the WhatsApp client on the test App.
func withWhatsApp(wa *whatsapp.Client) appOption {
	return func(a *handlers.App) {
		a.WhatsApp = wa
	}
}

// withPerms gives a fixture user a role in orgID that holds exactly the given
// "resource:action" keys. Use it for tests that exercise a handler behind a
// permission check rather than the check itself.
//
// The role and its role_permissions rows are inserted directly (no upsert of
// the shared permissions table), so parallel tests do not contend on it.
func withPerms(t *testing.T, app *handlers.App, orgID uuid.UUID, keys ...string) testutil.UserOption {
	t.Helper()
	testutil.GetOrCreateTestPermissions(t, app.DB)

	var perms []models.Permission
	require.NoError(t, app.DB.Where("resource || ':' || action IN ?", keys).Find(&perms).Error)
	require.Len(t, perms, len(keys), "unknown permission key in %v", keys)

	role := &models.CustomRole{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: orgID,
		Name:           "fixture_" + uuid.NewString()[:8],
	}
	require.NoError(t, app.DB.Omit("Permissions").Create(role).Error)
	links := make([]models.RolePermission, len(perms))
	for i, p := range perms {
		links[i] = models.RolePermission{CustomRoleID: role.ID, PermissionID: p.ID}
	}
	require.NoError(t, app.DB.Create(&links).Error)
	return testutil.WithRoleID(&role.ID)
}

// withHTTPClient sets the HTTP client on the test App.
func withHTTPClient(client *http.Client) appOption {
	return func(a *handlers.App) {
		a.HTTPClient = client
	}
}

// newTestApp creates an App instance for testing with a test database, Redis, and default config.
// Skips the test if TEST_REDIS_URL is not set.
func newTestApp(t *testing.T, opts ...appOption) *handlers.App {
	t.Helper()

	db := testutil.SetupTestDB(t)
	log := testutil.NopLogger()

	redisClient := testutil.SetupTestRedis(t)
	if redisClient == nil {
		t.Skip("TEST_REDIS_URL not set, skipping test")
	}

	cfg := &config.Config{
		App: config.AppConfig{
			EncryptionKey: "test-encryption-key-for-handlers-longer-than-32-chars",
		},
		JWT: config.JWTConfig{
			Secret:            testutil.TestJWTSecret,
			AccessExpiryMins:  15,
			RefreshExpiryDays: 7,
		},
	}

	app := &handlers.App{
		Config: cfg,
		DB:     db,
		Log:    log,
		Redis:  redisClient,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(app)
	}

	return app
}

// newIsolatedTestApp is newTestApp backed by a private, freshly migrated
// database schema (see testutil.SetupIsolatedTestDB). Use it for handlers whose
// behaviour is deliberately global — super-admin listings and cross-org chat
// repair scans — so sibling tests' rows can neither leak into assertions nor be
// mutated by the code under test.
func newIsolatedTestApp(t *testing.T, opts ...appOption) *handlers.App {
	t.Helper()

	app := newTestApp(t, opts...)
	app.DB = testutil.SetupIsolatedTestDB(t)
	return app
}
