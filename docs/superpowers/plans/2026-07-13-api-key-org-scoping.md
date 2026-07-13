# API Key Org Scoping & Super Admin Keys Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show which organisation an API key is being created for, and let super admins create either an org-specific key or a platform-wide "Super Admin" key with full cross-org access.

**Architecture:** Go backend (fastglue + GORM/Postgres) exposes `/api-keys` CRUD handlers in `internal/handlers/apikeys.go`; a Vue 3 + Pinia frontend (`frontend/src/views/settings/APIKeysView.vue`) drives the UI. The `api_keys.organization_id` column becomes nullable and a new `is_super_admin_key` boolean flags platform-wide keys. Super-admin org context resolution reuses the existing `X-Organization-ID` header override machinery already used for super-admin JWT sessions (`getOrgID` in `internal/handlers/app.go`) — no new override mechanism is introduced.

**Tech Stack:** Go 1.x, fastglue, GORM (Postgres), Vue 3 (`<script setup>`), Pinia, vue-i18n, Playwright (e2e).

## Global Constraints

- Non-super-admin users can never set `organization_id` or `is_super_admin_key` on a create request — these fields are silently ignored server-side, key is always scoped to the caller's own org (or the org they've switched into via existing membership rules). This is a security invariant, not a UX choice.
- A super-admin API key carries the same reach as a super-admin JWT session (full cross-org access via `X-Organization-ID` header) — no new privilege tier, only a new credential type.
- `api_keys.organization_id` is `NULL` if and only if `api_keys.is_super_admin_key` is `true`. This invariant is enforced in the handler, not at the DB layer.
- i18n: add new UI strings to `frontend/src/i18n/locales/en.json` only. The project has `es.json`/`hi.json`/`ta.json` but translating this feature's strings into all locales is out of scope for this fix — vue-i18n falls back to English for missing keys.
- No `.sql` migration files exist for `api_keys` today (GORM `AutoMigrate` owns the schema) — stay consistent with that pattern; do not introduce a new migration file.
- Backend tests require `TEST_DATABASE_URL` to be set (they auto-skip otherwise, per `test/testutil/db.go`). Run backend tests with that variable set in your shell.

---

### Task 1: Data model — nullable org + super-admin key flag

**Files:**
- Modify: `internal/models/models.go:215-234` (`APIKey` struct)
- Modify: `internal/handlers/apikeys.go:152-160` (fix compile break only — `CreateAPIKey` is fully rewritten in Task 2)
- Modify: `internal/middleware/middleware.go:242-252` (`validateAPIKey`)
- Modify: `internal/handlers/apikeys_test.go:33-48,159,478` (`createTestAPIKey` helper + two assertions)

**Interfaces:**
- Produces: `models.APIKey.OrganizationID` is now `*uuid.UUID` (nil only for platform-wide keys). `models.APIKey.IsSuperAdminKey bool` is new. Every later task builds on this.

- [ ] **Step 1: Update the `APIKey` model**

Replace `internal/models/models.go:215-234` with:

```go
// APIKey represents an API key for programmatic access.
// OrganizationID is nil only when IsSuperAdminKey is true (a platform-wide
// key not tied to any single organisation).
type APIKey struct {
	BaseModel
	OrganizationID  *uuid.UUID `gorm:"type:uuid;index" json:"organization_id,omitempty"`
	UserID          uuid.UUID  `gorm:"type:uuid;index;not null" json:"user_id"` // Creator
	Name            string     `gorm:"size:255;not null" json:"name"`
	KeyPrefix       string     `gorm:"size:16;index" json:"key_prefix"` // First 16 chars for identification
	KeyHash         string     `gorm:"size:255;not null" json:"-"`      // bcrypt hash of full key
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"` // null = never expires
	IsActive        bool       `gorm:"default:true" json:"is_active"`
	IsSuperAdminKey bool       `gorm:"default:false;not null" json:"is_super_admin_key"`

	// Relations
	Organization *Organization `gorm:"foreignKey:OrganizationID" json:"organization,omitempty"`
	User         *User         `gorm:"foreignKey:UserID" json:"user,omitempty"`
}
```

- [ ] **Step 2: Fix the resulting compile break in `CreateAPIKey`**

In `internal/handlers/apikeys.go`, inside `CreateAPIKey`, change:

```go
	apiKey := models.APIKey{
		OrganizationID: orgID,
```

to:

```go
	apiKey := models.APIKey{
		OrganizationID: &orgID,
```

(Task 2 replaces the whole `CreateAPIKey` function body, including this line, with the full org-resolution logic — this step only keeps the package compiling in the meantime.)

- [ ] **Step 3: Fix `validateAPIKey` to not stamp a fixed org for platform-wide keys**

In `internal/middleware/middleware.go`, inside `validateAPIKey`, change:

```go
			// Set context values from the user who created the key
			if apiKey.User != nil {
				r.RequestCtx.SetUserValue(ContextKeyUserID, apiKey.UserID)
				r.RequestCtx.SetUserValue(ContextKeyOrganizationID, apiKey.OrganizationID)
				r.RequestCtx.SetUserValue(ContextKeyEmail, apiKey.User.Email)
```

to:

```go
			// Set context values from the user who created the key
			if apiKey.User != nil {
				r.RequestCtx.SetUserValue(ContextKeyUserID, apiKey.UserID)
				if apiKey.OrganizationID != nil {
					r.RequestCtx.SetUserValue(ContextKeyOrganizationID, *apiKey.OrganizationID)
				}
				r.RequestCtx.SetUserValue(ContextKeyEmail, apiKey.User.Email)
```

A platform-wide (super-admin) key therefore leaves `ContextKeyOrganizationID` unset. Downstream, `getOrgID` (`internal/handlers/app.go:61`) already errors with "organization_id not found in context" when that value is absent, and already lets super admins supply `X-Organization-ID` to pick a target org — so a super-admin key must pass that header on every org-scoped request, identical to today's super-admin JWT session UX. No new code path is needed for that part.

- [ ] **Step 4: Update the test helper and assertions in `apikeys_test.go`**

Replace `createTestAPIKey` (`internal/handlers/apikeys_test.go:33-48`) with:

```go
// createTestAPIKey creates a test API key directly in the database.
func createTestAPIKey(t *testing.T, app *handlers.App, orgID, userID uuid.UUID, name string) *models.APIKey {
	t.Helper()

	orgIDCopy := orgID
	apiKey := &models.APIKey{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: &orgIDCopy,
		UserID:         userID,
		Name:           name,
		KeyPrefix:      "abcd1234",
		KeyHash:        "$2a$10$dummyhashvaluefortesting000000000000000000000000000000",
		IsActive:       true,
	}
	require.NoError(t, app.DB.Create(apiKey).Error)
	return apiKey
}

// createTestSuperAdminAPIKey creates a platform-wide (org-less) test API key.
func createTestSuperAdminAPIKey(t *testing.T, app *handlers.App, userID uuid.UUID, name string) *models.APIKey {
	t.Helper()

	apiKey := &models.APIKey{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  nil,
		UserID:          userID,
		Name:            name,
		KeyPrefix:       "ffff0000",
		KeyHash:         "$2a$10$dummyhashvaluefortesting000000000000000000000000000000",
		IsActive:        true,
		IsSuperAdminKey: true,
	}
	require.NoError(t, app.DB.Create(apiKey).Error)
	return apiKey
}
```

Then fix the two assertions that read `dbKey.OrganizationID` as a value type. At `internal/handlers/apikeys_test.go:159` and `internal/handlers/apikeys_test.go:478`, change:

```go
	assert.Equal(t, org.ID, dbKey.OrganizationID)
```

to:

```go
	require.NotNil(t, dbKey.OrganizationID)
	assert.Equal(t, org.ID, *dbKey.OrganizationID)
```

- [ ] **Step 5: Verify the whole module still builds**

Run: `go build ./...`
Expected: exits with no output/errors.

- [ ] **Step 6: Run the existing API key test suite**

Run (with `TEST_DATABASE_URL` set): `go test ./internal/handlers/... -run TestApp_.*APIKey -v`
Expected: PASS — all existing tests still pass unchanged (behavior hasn't changed yet, only the storage type).

- [ ] **Step 7: Commit**

```bash
git add internal/models/models.go internal/handlers/apikeys.go internal/middleware/middleware.go internal/handlers/apikeys_test.go
git commit -m "feat(api-keys): make organization_id nullable, add is_super_admin_key flag"
```

---

### Task 2: `CreateAPIKey` — org resolution and super-admin key creation

**Files:**
- Modify: `internal/handlers/apikeys.go:16-19` (`APIKeyRequest`)
- Modify: `internal/handlers/apikeys.go:105-176` (`CreateAPIKey`)
- Modify: `internal/handlers/apikeys_test.go` (new tests, appended after `TestApp_CreateAPIKey_InvalidJSONBody`)

**Interfaces:**
- Consumes: `models.APIKey{OrganizationID *uuid.UUID, IsSuperAdminKey bool, ...}` (Task 1), `a.IsSuperAdmin(userID uuid.UUID) bool` (`internal/handlers/cache.go:536`), `a.HasPermission(userID uuid.UUID, resource, action string, orgIDs ...uuid.UUID) bool` (`internal/handlers/cache.go:484`), `a.getOrgID(r *fastglue.Request) (uuid.UUID, error)` (`internal/handlers/app.go:61`).
- Produces: `APIKeyRequest{Name string, ExpiresAt *string, OrganizationID *uuid.UUID, IsSuperAdminKey bool}` — consumed by nothing else yet, but this is the request shape the frontend (Task 6) targets.

- [ ] **Step 1: Add the new request fields**

In `internal/handlers/apikeys.go`, replace `APIKeyRequest` (lines 16-19):

```go
// APIKeyRequest represents the request body for creating an API key
type APIKeyRequest struct {
	Name            string     `json:"name"`
	ExpiresAt       *string    `json:"expires_at,omitempty"`
	OrganizationID  *uuid.UUID `json:"organization_id,omitempty"`
	IsSuperAdminKey bool       `json:"is_super_admin_key,omitempty"`
}
```

- [ ] **Step 2: Write failing test — super admin creates a platform-wide key**

Append to `internal/handlers/apikeys_test.go` (after `TestApp_CreateAPIKey_InvalidJSONBody`):

```go
func TestApp_CreateAPIKey_SuperAdmin_PlatformKey(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	admin := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithEmail(testutil.UniqueEmail("super-create-platform")), testutil.WithSuperAdmin())

	req := testutil.NewJSONRequest(t, map[string]any{
		"name":                "Platform Key",
		"is_super_admin_key": true,
	})
	testutil.SetAuthContext(req, org.ID, admin.ID)

	err := app.CreateAPIKey(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp struct {
		Data handlers.APIKeyCreateResponse `json:"data"`
	}
	err = json.Unmarshal(testutil.GetResponseBody(req), &resp)
	require.NoError(t, err)

	var dbKey models.APIKey
	err = app.DB.Where("id = ?", resp.Data.ID).First(&dbKey).Error
	require.NoError(t, err)
	assert.Nil(t, dbKey.OrganizationID)
	assert.True(t, dbKey.IsSuperAdminKey)
	assert.Equal(t, admin.ID, dbKey.UserID)
}

func TestApp_CreateAPIKey_SuperAdmin_SpecificOrg(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	adminOrg := testutil.CreateTestOrganization(t, app.DB)
	targetOrg := testutil.CreateTestOrganization(t, app.DB)
	admin := testutil.CreateTestUser(t, app.DB, adminOrg.ID, testutil.WithEmail(testutil.UniqueEmail("super-create-org")), testutil.WithSuperAdmin())

	req := testutil.NewJSONRequest(t, map[string]any{
		"name":            "Org Specific Key",
		"organization_id": targetOrg.ID.String(),
	})
	testutil.SetAuthContext(req, adminOrg.ID, admin.ID)

	err := app.CreateAPIKey(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp struct {
		Data handlers.APIKeyCreateResponse `json:"data"`
	}
	err = json.Unmarshal(testutil.GetResponseBody(req), &resp)
	require.NoError(t, err)

	var dbKey models.APIKey
	err = app.DB.Where("id = ?", resp.Data.ID).First(&dbKey).Error
	require.NoError(t, err)
	require.NotNil(t, dbKey.OrganizationID)
	assert.Equal(t, targetOrg.ID, *dbKey.OrganizationID)
	assert.False(t, dbKey.IsSuperAdminKey)
}

func TestApp_CreateAPIKey_NonSuperAdmin_IgnoresOrgFields(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	otherOrg := testutil.CreateTestOrganization(t, app.DB)
	perms := getAPIKeyPermissions(t, app)
	role := testutil.CreateTestRoleExact(t, app.DB, org.ID, "API Key Creator Ignore", false, false, perms)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithEmail(testutil.UniqueEmail("create-ignore-org")), testutil.WithRoleID(&role.ID))

	req := testutil.NewJSONRequest(t, map[string]any{
		"name":                "Should Stay In Own Org",
		"organization_id":     otherOrg.ID.String(),
		"is_super_admin_key": true,
	})
	testutil.SetAuthContext(req, org.ID, user.ID)

	err := app.CreateAPIKey(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp struct {
		Data handlers.APIKeyCreateResponse `json:"data"`
	}
	err = json.Unmarshal(testutil.GetResponseBody(req), &resp)
	require.NoError(t, err)

	var dbKey models.APIKey
	err = app.DB.Where("id = ?", resp.Data.ID).First(&dbKey).Error
	require.NoError(t, err)
	require.NotNil(t, dbKey.OrganizationID)
	assert.Equal(t, org.ID, *dbKey.OrganizationID)
	assert.False(t, dbKey.IsSuperAdminKey)
}
```

- [ ] **Step 3: Run the new tests to verify they fail**

Run: `go test ./internal/handlers/... -run 'TestApp_CreateAPIKey_SuperAdmin_PlatformKey|TestApp_CreateAPIKey_SuperAdmin_SpecificOrg|TestApp_CreateAPIKey_NonSuperAdmin_IgnoresOrgFields' -v`
Expected: FAIL — `dbKey.OrganizationID` is not nil for the platform-key test (handler still always assigns the caller's own org), and the org-specific/ignore tests fail because the handler doesn't read `OrganizationID`/`IsSuperAdminKey` from the request yet.

- [ ] **Step 4: Implement the new `CreateAPIKey`**

Replace `CreateAPIKey` (`internal/handlers/apikeys.go:105-176`) with:

```go
// CreateAPIKey creates a new API key.
// Regular users always get a key scoped to their own organization; any
// organization_id/is_super_admin_key fields they send are ignored.
// Super admins may create a key scoped to any organization, or a platform-wide
// super-admin key (is_super_admin_key: true) that is not tied to any org.
func (a *App) CreateAPIKey(r *fastglue.Request) error {
	userID, ok := r.RequestCtx.UserValue("user_id").(uuid.UUID)
	if !ok {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	isSuperAdmin := a.IsSuperAdmin(userID)

	var req APIKeyRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	if req.Name == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Name is required", nil, "")
	}

	var targetOrgID *uuid.UUID
	isSuperAdminKey := false

	switch {
	case isSuperAdmin && req.IsSuperAdminKey:
		isSuperAdminKey = true
	case isSuperAdmin && req.OrganizationID != nil:
		var count int64
		if err := a.DB.Table("organizations").Where("id = ?", *req.OrganizationID).Count(&count).Error; err != nil || count == 0 {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Organization not found", nil, "")
		}
		targetOrgID = req.OrganizationID
	default:
		orgID, err := a.getOrgID(r)
		if err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "organization_id or is_super_admin_key is required", nil, "")
		}
		targetOrgID = &orgID
	}

	if !isSuperAdmin {
		if targetOrgID == nil || !a.HasPermission(userID, models.ResourceAPIKeys, models.ActionWrite, *targetOrgID) {
			return r.SendErrorEnvelope(fasthttp.StatusForbidden, "Insufficient permissions", nil, "")
		}
	}

	// Parse expiration date if provided
	var expiresAt *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid expires_at format. Use RFC3339 format", nil, "")
		}
		expiresAt = &t
	}

	// Generate the API key
	fullKey, err := generateAPIKey()
	if err != nil {
		a.Log.Error("Failed to generate API key", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to generate API key", nil, "")
	}

	// Hash the key for storage
	hashedKey, err := bcrypt.GenerateFromPassword([]byte(fullKey), bcrypt.DefaultCost)
	if err != nil {
		a.Log.Error("Failed to hash API key", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to create API key", nil, "")
	}

	// Extract prefix (first 16 chars after "whm_")
	keyPrefix := fullKey[4:20]

	apiKey := models.APIKey{
		OrganizationID:  targetOrgID,
		UserID:          userID,
		Name:            req.Name,
		KeyPrefix:       keyPrefix,
		KeyHash:         string(hashedKey),
		ExpiresAt:       expiresAt,
		IsActive:        true,
		IsSuperAdminKey: isSuperAdminKey,
	}

	if err := a.DB.Create(&apiKey).Error; err != nil {
		a.Log.Error("Failed to create API key", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to create API key", nil, "")
	}

	// Return full key only on creation
	return r.SendEnvelope(APIKeyCreateResponse{
		ID:        apiKey.ID,
		Name:      apiKey.Name,
		Key:       fullKey, // This is the only time the full key is returned
		KeyPrefix: apiKey.KeyPrefix,
		ExpiresAt: apiKey.ExpiresAt,
		CreatedAt: apiKey.CreatedAt.Format("2006-01-02T15:04:05Z"),
	})
}
```

- [ ] **Step 5: Run the new tests to verify they pass, and the full file for regressions**

Run: `go test ./internal/handlers/... -run TestApp_CreateAPIKey -v`
Expected: PASS — all `TestApp_CreateAPIKey*` tests (existing and new) pass.

- [ ] **Step 6: Commit**

```bash
git add internal/handlers/apikeys.go internal/handlers/apikeys_test.go
git commit -m "feat(api-keys): let super admins create org-specific or platform-wide keys"
```

---

### Task 3: `ListAPIKeys` — cross-org listing for super admins, org name/badge fields

**Files:**
- Modify: `internal/handlers/apikeys.go:22-30` (`APIKeyResponse`)
- Modify: `internal/handlers/apikeys.go:52-102` (`ListAPIKeys`)
- Modify: `internal/handlers/apikeys_test.go` (new tests, appended after `TestApp_ListAPIKeys_ExcludesDeletedKeys`)

**Interfaces:**
- Consumes: `createTestSuperAdminAPIKey` (Task 1), `models.APIKey.Organization *models.Organization` (existing relation, `Organization.Name string`).
- Produces: `APIKeyResponse{..., OrganizationName string, IsSuperAdminKey bool}` — this is the shape the frontend list table (Task 8) reads.

- [ ] **Step 1: Add the new response fields**

Replace `APIKeyResponse` (`internal/handlers/apikeys.go:22-30`):

```go
// APIKeyResponse represents an API key in list responses
type APIKeyResponse struct {
	ID               uuid.UUID  `json:"id"`
	Name             string     `json:"name"`
	KeyPrefix        string     `json:"key_prefix"`
	LastUsedAt       *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	IsActive         bool       `json:"is_active"`
	CreatedAt        string     `json:"created_at"`
	OrganizationName string     `json:"organization_name,omitempty"`
	IsSuperAdminKey  bool       `json:"is_super_admin_key"`
}
```

- [ ] **Step 2: Write failing tests**

Append to `internal/handlers/apikeys_test.go` (after `TestApp_ListAPIKeys_ExcludesDeletedKeys`):

```go
func TestApp_ListAPIKeys_SuperAdmin_SeesAllOrgs(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	org1 := testutil.CreateTestOrganization(t, app.DB)
	org2 := testutil.CreateTestOrganization(t, app.DB)
	admin := testutil.CreateTestUser(t, app.DB, org1.ID, testutil.WithEmail(testutil.UniqueEmail("list-super-admin")), testutil.WithSuperAdmin())

	createTestAPIKey(t, app, org1.ID, admin.ID, "Org1 Key")
	createTestAPIKey(t, app, org2.ID, admin.ID, "Org2 Key")
	createTestSuperAdminAPIKey(t, app, admin.ID, "Platform Key")

	req := testutil.NewGETRequest(t)
	testutil.SetAuthContext(req, org1.ID, admin.ID)

	err := app.ListAPIKeys(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp struct {
		Data struct {
			APIKeys []handlers.APIKeyResponse `json:"api_keys"`
			Total   int                       `json:"total"`
		} `json:"data"`
	}
	err = json.Unmarshal(testutil.GetResponseBody(req), &resp)
	require.NoError(t, err)
	assert.Equal(t, 3, resp.Data.Total)

	var sawOrg1, sawOrg2, sawPlatform bool
	for _, k := range resp.Data.APIKeys {
		switch k.Name {
		case "Org1 Key":
			sawOrg1 = true
			assert.Equal(t, org1.Name, k.OrganizationName)
			assert.False(t, k.IsSuperAdminKey)
		case "Org2 Key":
			sawOrg2 = true
			assert.Equal(t, org2.Name, k.OrganizationName)
		case "Platform Key":
			sawPlatform = true
			assert.True(t, k.IsSuperAdminKey)
			assert.Empty(t, k.OrganizationName)
		}
	}
	assert.True(t, sawOrg1 && sawOrg2 && sawPlatform)
}

func TestApp_ListAPIKeys_ResponseIncludesOrgName(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	perms := getAPIKeyPermissions(t, app)
	role := testutil.CreateTestRoleExact(t, app.DB, org.ID, "API Key Reader OrgName", false, false, perms)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithEmail(testutil.UniqueEmail("list-orgname")), testutil.WithRoleID(&role.ID))

	createTestAPIKey(t, app, org.ID, user.ID, "Named Org Key")

	req := testutil.NewGETRequest(t)
	testutil.SetAuthContext(req, org.ID, user.ID)

	err := app.ListAPIKeys(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp struct {
		Data struct {
			APIKeys []handlers.APIKeyResponse `json:"api_keys"`
		} `json:"data"`
	}
	err = json.Unmarshal(testutil.GetResponseBody(req), &resp)
	require.NoError(t, err)
	require.Len(t, resp.Data.APIKeys, 1)
	assert.Equal(t, org.Name, resp.Data.APIKeys[0].OrganizationName)
	assert.False(t, resp.Data.APIKeys[0].IsSuperAdminKey)
}
```

- [ ] **Step 3: Run the new tests to verify they fail**

Run: `go test ./internal/handlers/... -run 'TestApp_ListAPIKeys_SuperAdmin_SeesAllOrgs|TestApp_ListAPIKeys_ResponseIncludesOrgName' -v`
Expected: FAIL — `OrganizationName` is empty (field doesn't exist in the handler's mapping yet), and the super-admin test only sees 1 key (list is still hard-filtered to the caller's org).

- [ ] **Step 4: Implement the new `ListAPIKeys`**

Replace `ListAPIKeys` (`internal/handlers/apikeys.go:52-102`) with:

```go
// ListAPIKeys returns API keys. Regular users see only their own organization's
// keys; super admins see every key across every organization plus platform-wide
// super-admin keys.
func (a *App) ListAPIKeys(r *fastglue.Request) error {
	userID, ok := r.RequestCtx.UserValue("user_id").(uuid.UUID)
	if !ok {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	isSuperAdmin := a.IsSuperAdmin(userID)

	if !isSuperAdmin {
		if err := a.requirePermission(r, userID, models.ResourceAPIKeys, models.ActionRead); err != nil {
			return nil
		}
	}

	pg := parsePagination(r)
	search := string(r.RequestCtx.QueryArgs().Peek("search"))

	query := a.DB.Model(&models.APIKey{}).Preload("Organization")
	if !isSuperAdmin {
		orgID, err := a.getOrgID(r)
		if err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
		}
		query = query.Where("organization_id = ?", orgID)
	}

	// Apply search filter - search by name or key prefix (case-insensitive)
	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("name ILIKE ? OR key_prefix ILIKE ?", searchPattern, searchPattern)
	}

	var total int64
	query.Count(&total)

	var apiKeys []models.APIKey
	if err := pg.Apply(query.Order("created_at DESC")).
		Find(&apiKeys).Error; err != nil {
		a.Log.Error("Failed to list API keys", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to list API keys", nil, "")
	}

	response := make([]APIKeyResponse, len(apiKeys))
	for i, key := range apiKeys {
		orgName := ""
		if key.Organization != nil {
			orgName = key.Organization.Name
		}
		response[i] = APIKeyResponse{
			ID:               key.ID,
			Name:             key.Name,
			KeyPrefix:        key.KeyPrefix,
			LastUsedAt:       key.LastUsedAt,
			ExpiresAt:        key.ExpiresAt,
			IsActive:         key.IsActive,
			CreatedAt:        key.CreatedAt.Format("2006-01-02T15:04:05Z"),
			OrganizationName: orgName,
			IsSuperAdminKey:  key.IsSuperAdminKey,
		}
	}

	return r.SendEnvelope(map[string]any{
		"api_keys": response,
		"total":    total,
		"page":     pg.Page,
		"limit":    pg.Limit,
	})
}
```

- [ ] **Step 5: Run the full list-test suite to verify pass with no regressions**

Run: `go test ./internal/handlers/... -run TestApp_ListAPIKeys -v`
Expected: PASS — all `TestApp_ListAPIKeys*` tests (existing and new) pass.

- [ ] **Step 6: Commit**

```bash
git add internal/handlers/apikeys.go internal/handlers/apikeys_test.go
git commit -m "feat(api-keys): show org name in list, let super admins list keys across all orgs"
```

---

### Task 4: `DeleteAPIKey` — cross-org delete for super admins

**Files:**
- Modify: `internal/handlers/apikeys.go:179-204` (`DeleteAPIKey`)
- Modify: `internal/handlers/apikeys_test.go` (new tests, appended at end of file)

**Interfaces:**
- Consumes: same as Task 3.
- Produces: nothing new consumed elsewhere.

- [ ] **Step 1: Write failing tests**

Append to `internal/handlers/apikeys_test.go` (end of file, after the closing brace of `TestApp_DeleteAPIKey`):

```go
func TestApp_DeleteAPIKey_SuperAdmin_CanDeleteAnyOrgKey(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	org1 := testutil.CreateTestOrganization(t, app.DB)
	org2 := testutil.CreateTestOrganization(t, app.DB)
	owner := testutil.CreateTestUser(t, app.DB, org1.ID, testutil.WithEmail(testutil.UniqueEmail("del-super-owner")))
	admin := testutil.CreateTestUser(t, app.DB, org2.ID, testutil.WithEmail(testutil.UniqueEmail("del-super-admin")), testutil.WithSuperAdmin())

	apiKey := createTestAPIKey(t, app, org1.ID, owner.ID, "Org1 Key To Delete")

	req := testutil.NewGETRequest(t)
	testutil.SetAuthContext(req, org2.ID, admin.ID)
	testutil.SetPathParam(req, "id", apiKey.ID.String())

	err := app.DeleteAPIKey(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var count int64
	app.DB.Model(&models.APIKey{}).Where("id = ?", apiKey.ID).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestApp_DeleteAPIKey_SuperAdmin_CanDeletePlatformKey(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	admin := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithEmail(testutil.UniqueEmail("del-platform-admin")), testutil.WithSuperAdmin())

	apiKey := createTestSuperAdminAPIKey(t, app, admin.ID, "Platform Key To Delete")

	req := testutil.NewGETRequest(t)
	testutil.SetAuthContext(req, org.ID, admin.ID)
	testutil.SetPathParam(req, "id", apiKey.ID.String())

	err := app.DeleteAPIKey(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var count int64
	app.DB.Model(&models.APIKey{}).Where("id = ?", apiKey.ID).Count(&count)
	assert.Equal(t, int64(0), count)
}
```

- [ ] **Step 2: Run the new tests to verify they fail**

Run: `go test ./internal/handlers/... -run 'TestApp_DeleteAPIKey_SuperAdmin' -v`
Expected: FAIL — both return 404, because `DeleteAPIKey` still hard-scopes the delete to the caller's own `organization_id`, which doesn't match `org1` (first test) or is `nil` on the platform key (second test).

- [ ] **Step 3: Implement the new `DeleteAPIKey`**

Replace `DeleteAPIKey` (`internal/handlers/apikeys.go:179-204`) with:

```go
// DeleteAPIKey revokes an API key. Regular users may only delete keys within
// their own organization; super admins may delete any key, including
// platform-wide super-admin keys.
func (a *App) DeleteAPIKey(r *fastglue.Request) error {
	userID, ok := r.RequestCtx.UserValue("user_id").(uuid.UUID)
	if !ok {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	isSuperAdmin := a.IsSuperAdmin(userID)

	if !isSuperAdmin {
		if err := a.requirePermission(r, userID, models.ResourceAPIKeys, models.ActionDelete); err != nil {
			return nil
		}
	}

	id, err := parsePathUUID(r, "id", "API key")
	if err != nil {
		return nil
	}

	query := a.DB.Where("id = ?", id)
	if !isSuperAdmin {
		orgID, err := a.getOrgID(r)
		if err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
		}
		query = query.Where("organization_id = ?", orgID)
	}

	result := query.Delete(&models.APIKey{})
	if result.Error != nil {
		a.Log.Error("Failed to delete API key", "error", result.Error)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to delete API key", nil, "")
	}
	if result.RowsAffected == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "API key not found", nil, "")
	}

	return r.SendEnvelope(map[string]string{"message": "API key deleted successfully"})
}
```

- [ ] **Step 4: Run the full delete-test suite to verify pass with no regressions**

Run: `go test ./internal/handlers/... -run TestApp_DeleteAPIKey -v`
Expected: PASS — all `TestApp_DeleteAPIKey*` tests (existing and new) pass, including the existing `cross-org isolation` test (a non-super-admin still gets 404 deleting another org's key).

- [ ] **Step 5: Commit**

```bash
git add internal/handlers/apikeys.go internal/handlers/apikeys_test.go
git commit -m "feat(api-keys): let super admins delete any org's key or a platform key"
```

---

### Task 5: Middleware regression coverage for API-key org context

**Files:**
- Modify: `internal/middleware/middleware_test.go` (new tests, appended at end of file)

**Interfaces:**
- Consumes: `middleware.AuthWithDB(secret string, db *gorm.DB) fastglue.FastMiddleware` (`internal/middleware/middleware.go:143`), `middleware.ContextKeyUserID/ContextKeyOrganizationID/ContextKeyIsSuperAdmin` (exported string constants, `internal/middleware/middleware.go:22-26`), `testutil.SetupTestDB(t) *gorm.DB` (`test/testutil/db.go:25`).
- Produces: nothing new consumed elsewhere — this task only adds coverage for the behavior Task 1 already implemented.

This task verifies the security-critical behavior changed in Task 1 (Step 3): a platform-wide super-admin key must not stamp a fixed org into the request context, while an ordinary org-scoped key still does.

- [ ] **Step 1: Add required imports**

In `internal/middleware/middleware_test.go`, add `"strings"` and `"golang.org/x/crypto/bcrypt"` to the import block:

```go
import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/middleware"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"golang.org/x/crypto/bcrypt"
)
```

- [ ] **Step 2: Write the regression tests**

Append to `internal/middleware/middleware_test.go`:

```go
func TestAuthWithDB_APIKey_SuperAdminKey_OmitsOrgContext(t *testing.T) {
	db := testutil.SetupTestDB(t)

	org := testutil.CreateTestOrganization(t, db)
	admin := testutil.CreateTestUser(t, db, org.ID, testutil.WithEmail(testutil.UniqueEmail("mw-super-admin")), testutil.WithSuperAdmin())

	plainKey := "whm_" + strings.Repeat("a", 32)
	hash, err := bcrypt.GenerateFromPassword([]byte(plainKey), bcrypt.DefaultCost)
	require.NoError(t, err)

	apiKey := &testutil.APIKeyFixture{
		ID:              uuid.New(),
		OrganizationID:  nil,
		UserID:          admin.ID,
		Name:            "Platform Key",
		KeyPrefix:       plainKey[4:20],
		KeyHash:         string(hash),
		IsActive:        true,
		IsSuperAdminKey: true,
	}
	testutil.CreateTestAPIKeyFixture(t, db, apiKey)

	req := newTestRequest()
	req.RequestCtx.Request.Header.Set("X-API-Key", plainKey)

	authMiddleware := middleware.AuthWithDB(testJWTSecret, db)
	result := authMiddleware(req)

	require.NotNil(t, result, "middleware should not reject a valid super-admin key")
	assert.Equal(t, admin.ID, result.RequestCtx.UserValue(middleware.ContextKeyUserID))
	assert.Equal(t, true, result.RequestCtx.UserValue(middleware.ContextKeyIsSuperAdmin))
	assert.Nil(t, result.RequestCtx.UserValue(middleware.ContextKeyOrganizationID), "super-admin key must not stamp a fixed org into context")
}

func TestAuthWithDB_APIKey_OrgScopedKey_SetsOrgContext(t *testing.T) {
	db := testutil.SetupTestDB(t)

	org := testutil.CreateTestOrganization(t, db)
	user := testutil.CreateTestUser(t, db, org.ID, testutil.WithEmail(testutil.UniqueEmail("mw-org-key")))

	plainKey := "whm_" + strings.Repeat("b", 32)
	hash, err := bcrypt.GenerateFromPassword([]byte(plainKey), bcrypt.DefaultCost)
	require.NoError(t, err)

	orgID := org.ID
	apiKey := &testutil.APIKeyFixture{
		ID:             uuid.New(),
		OrganizationID: &orgID,
		UserID:         user.ID,
		Name:           "Org Key",
		KeyPrefix:      plainKey[4:20],
		KeyHash:        string(hash),
		IsActive:       true,
	}
	testutil.CreateTestAPIKeyFixture(t, db, apiKey)

	req := newTestRequest()
	req.RequestCtx.Request.Header.Set("X-API-Key", plainKey)

	authMiddleware := middleware.AuthWithDB(testJWTSecret, db)
	result := authMiddleware(req)

	require.NotNil(t, result)
	assert.Equal(t, org.ID, result.RequestCtx.UserValue(middleware.ContextKeyOrganizationID))
}
```

- [ ] **Step 3: Add the `APIKeyFixture` test helper**

`internal/middleware/middleware_test.go` lives in package `middleware_test` and cannot import `internal/handlers` (would create an import cycle with `internal/middleware`), so it needs its own tiny DB-insert helper rather than reusing `internal/handlers`'s `createTestAPIKey`. Add to `test/testutil/fixtures.go` (end of file):

```go
// APIKeyFixture is the minimal set of fields needed to insert a test API key
// directly into the database from packages that cannot import internal/handlers
// or internal/models without an import cycle (e.g. internal/middleware tests).
type APIKeyFixture struct {
	ID              uuid.UUID
	OrganizationID  *uuid.UUID
	UserID          uuid.UUID
	Name            string
	KeyPrefix       string
	KeyHash         string
	IsActive        bool
	IsSuperAdminKey bool
}

// CreateTestAPIKeyFixture inserts an APIKeyFixture into the api_keys table.
func CreateTestAPIKeyFixture(t *testing.T, db *gorm.DB, f *APIKeyFixture) {
	t.Helper()

	require.NoError(t, db.Table("api_keys").Create(map[string]any{
		"id":                 f.ID,
		"organization_id":    f.OrganizationID,
		"user_id":            f.UserID,
		"name":               f.Name,
		"key_prefix":         f.KeyPrefix,
		"key_hash":           f.KeyHash,
		"is_active":          f.IsActive,
		"is_super_admin_key": f.IsSuperAdminKey,
	}).Error)
}
```

Note: `test/testutil` already imports `internal/models` for `runMigrations` (`test/testutil/db.go:9`), which is fine — the cycle risk is specifically `internal/middleware` (via its `_test` package) importing `internal/handlers`, not `test/testutil` importing `internal/models`.

- [ ] **Step 4: Run the new tests to verify they pass**

Run (with `TEST_DATABASE_URL` set): `go test ./internal/middleware/... -run TestAuthWithDB_APIKey -v`
Expected: PASS — both tests pass, confirming the Task 1 middleware fix behaves correctly (this is regression/characterization coverage for security-critical logic already implemented, not red-green TDD).

- [ ] **Step 5: Commit**

```bash
git add internal/middleware/middleware_test.go test/testutil/fixtures.go
git commit -m "test(middleware): cover org-context resolution for super-admin API keys"
```

---

### Task 6: Frontend — API types and org-store wiring

**Files:**
- Modify: `frontend/src/services/api.ts:199-205` (`apiKeysService`)
- Modify: `frontend/src/views/settings/APIKeysView.vue` (script: interfaces, stores, `fetchItems`, `createAPIKey`)

**Interfaces:**
- Consumes: `useAuthStore()` exposing `user.value.{is_super_admin?: boolean, organization_name?: string}` and `organizationId: ComputedRef<string>` (`frontend/src/stores/auth.ts:44-50,195`); `useOrganizationsStore()` exposing `organizations: Ref<Organization[]>` and `fetchOrganizations(): Promise<void>` (`frontend/src/stores/organizations.ts:7-8,35`).
- Produces: `apiKeysService.create(data: { name: string; expires_at?: string; organization_id?: string; is_super_admin_key?: boolean })` — consumed by Task 7's dialog submit handler (already wired in this task, but the org-picker UI is added in Task 7). `APIKey` interface gains `organization_name?: string` and `is_super_admin_key: boolean` — consumed by Task 8's table.

- [ ] **Step 1: Update `apiKeysService.create` signature**

In `frontend/src/services/api.ts`, replace lines 199-205:

```ts
export const apiKeysService = {
  list: (params?: { search?: string; page?: number; limit?: number }) =>
    api.get<{ api_keys: any[]; total?: number }>("/api-keys", { params }),
  create: (data: { name: string; expires_at?: string; organization_id?: string; is_super_admin_key?: boolean }) =>
    api.post("/api-keys", data),
  delete: (id: string) => api.delete(`/api-keys/${id}`),
};
```

- [ ] **Step 2: Update imports and interfaces in `APIKeysView.vue`**

In `frontend/src/views/settings/APIKeysView.vue`, add to the imports (after the existing `useViewRefresh` import at line 19):

```ts
import { useAuthStore } from '@/stores/auth'
import { useOrganizationsStore } from '@/stores/organizations'
```

Replace the `APIKey` and `APIKeyFormData` interfaces (lines 23-45):

```ts
interface APIKey {
  id: string
  name: string
  key_prefix: string
  last_used_at: string | null
  expires_at: string | null
  is_active: boolean
  created_at: string
  organization_name?: string
  is_super_admin_key: boolean
}

interface NewAPIKeyResponse {
  id: string
  name: string
  key: string
  key_prefix: string
  expires_at: string | null
  created_at: string
}

interface APIKeyFormData {
  name: string
  expires_at: string
  organization_id: string
  is_super_admin_key: boolean
}

const defaultFormData: APIKeyFormData = { name: '', expires_at: '', organization_id: '', is_super_admin_key: false }
```

- [ ] **Step 3: Add stores and `isSuperAdmin` computed**

After `const { t } = useI18n()` (line 21), add:

```ts
const authStore = useAuthStore()
const organizationsStore = useOrganizationsStore()
const isSuperAdmin = computed(() => authStore.user?.is_super_admin || false)
```

- [ ] **Step 4: Fetch organizations on mount for super admins**

Replace `onMounted(() => fetchItems())` (line 137) with:

```ts
onMounted(() => {
  fetchItems()
  if (isSuperAdmin.value) organizationsStore.fetchOrganizations()
})
```

- [ ] **Step 5: Send the new fields from `createAPIKey`**

Replace `createAPIKey` (lines 110-125):

```ts
async function createAPIKey() {
  if (!formData.value.name.trim()) { toast.error(t('apiKeys.nameRequired')); return }
  isSubmitting.value = true
  try {
    const payload: { name: string; expires_at?: string; organization_id?: string; is_super_admin_key?: boolean } = { name: formData.value.name.trim() }
    if (formData.value.expires_at) payload.expires_at = new Date(formData.value.expires_at).toISOString()
    if (isSuperAdmin.value && formData.value.is_super_admin_key) {
      payload.is_super_admin_key = true
    } else if (isSuperAdmin.value && formData.value.organization_id) {
      payload.organization_id = formData.value.organization_id
    }
    const response = await apiKeysService.create(payload)
    newlyCreatedKey.value = response.data.data
    closeCreateDialog()
    isKeyDisplayOpen.value = true
    formData.value = { ...defaultFormData }
    await fetchItems()
    toast.success(t('common.createdSuccess', { resource: t('resources.APIKey') }))
  } catch (error) { toast.error(getErrorMessage(error, t('common.failedCreate', { resource: t('resources.APIKey') }))) }
  finally { isSubmitting.value = false }
}
```

- [ ] **Step 6: Typecheck**

Run (from `frontend/`): `npm run typecheck`
Expected: exits with no errors. (`is_super_admin_key` on the fetched `APIKey` items will be `undefined` at runtime until Task 3's backend change ships — that's fine, it's optional-shaped in the type via the backend's `omitempty`-free `bool` default of `false`; the frontend field is typed as required `boolean` to match the backend response, so cast is not needed once Task 3 is deployed alongside this.)

- [ ] **Step 7: Commit**

```bash
git add frontend/src/services/api.ts frontend/src/views/settings/APIKeysView.vue
git commit -m "feat(api-keys): wire org fields into API key service and view state"
```

---

### Task 7: Frontend — create-dialog org display and super-admin picker

**Files:**
- Modify: `frontend/src/views/settings/APIKeysView.vue` (script: imports, `openCreateDialog` wrapper, template: create dialog, template: buttons)
- Modify: `frontend/src/i18n/locales/en.json:974-1005` (`apiKeys` block)

**Interfaces:**
- Consumes: `formData.value.{organization_id, is_super_admin_key}` (Task 6), `organizationsStore.organizations` (Task 6), `authStore.organizationId` (`frontend/src/stores/auth.ts:50`), `authStore.user?.organization_name` (`frontend/src/stores/auth.ts:34`).
- Produces: nothing new consumed by later tasks.

- [ ] **Step 1: Import the `Select` components**

In `frontend/src/views/settings/APIKeysView.vue`, add after the existing `Badge` import (line 8):

```ts
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
```

- [ ] **Step 2: Add an `openCreateDialog` wrapper that preselects the caller's own org**

After the `useCrudState` destructuring block (after line 52, before `const isKeyDisplayOpen = ref(false)`), add:

```ts
function openCreateDialog() {
  openCreateDialogBase()
  if (isSuperAdmin.value) {
    formData.value.organization_id = authStore.organizationId
  }
}
```

- [ ] **Step 3: Point both "create" buttons at the new wrapper**

In the template, change:

```html
<Button variant="outline" size="sm" @click="openCreateDialogBase"><Plus class="h-4 w-4 mr-2" />{{ $t('apiKeys.createApiKey') }}</Button>
```

(line 145, inside `PageHeader`'s `#actions` slot) to:

```html
<Button variant="outline" size="sm" @click="openCreateDialog"><Plus class="h-4 w-4 mr-2" />{{ $t('apiKeys.createApiKey') }}</Button>
```

And change the empty-state button (line 177, inside `#empty-action`):

```html
<Button variant="outline" size="sm" @click="openCreateDialogBase"><Plus class="h-4 w-4 mr-2" />{{ $t('apiKeys.createApiKey') }}</Button>
```

to:

```html
<Button variant="outline" size="sm" @click="openCreateDialog"><Plus class="h-4 w-4 mr-2" />{{ $t('apiKeys.createApiKey') }}</Button>
```

- [ ] **Step 4: Add the org label/picker to the create dialog**

Replace the `CrudFormDialog` block (lines 186-195):

```html
    <CrudFormDialog v-model:open="isCreateDialogOpen" :is-editing="false" :is-submitting="isSubmitting" :create-title="$t('apiKeys.createTitle')" :create-description="$t('apiKeys.createDesc')" :create-submit-label="$t('apiKeys.createSubmit')" @submit="createAPIKey">
      <div class="space-y-4">
        <div class="space-y-2"><Label for="name">{{ $t('apiKeys.name') }}</Label><Input id="name" v-model="formData.name" :placeholder="$t('apiKeys.namePlaceholder')" /></div>
        <div v-if="!isSuperAdmin" class="space-y-2">
          <Label>{{ $t('apiKeys.organization') }}</Label>
          <p class="text-sm text-muted-foreground">{{ $t('apiKeys.creatingKeyFor', { org: authStore.user?.organization_name || '' }) }}</p>
        </div>
        <div v-else class="space-y-2">
          <Label for="key-org">{{ $t('apiKeys.organization') }}</Label>
          <Select :model-value="formData.is_super_admin_key ? 'super_admin' : formData.organization_id" @update:model-value="(val) => { if (val === 'super_admin') { formData.is_super_admin_key = true; formData.organization_id = '' } else { formData.is_super_admin_key = false; formData.organization_id = String(val) } }">
            <SelectTrigger id="key-org"><SelectValue :placeholder="$t('apiKeys.organizationPlaceholder')" /></SelectTrigger>
            <SelectContent>
              <SelectItem value="super_admin">{{ $t('apiKeys.superAdminAllOrgs') }}</SelectItem>
              <SelectItem v-for="org in organizationsStore.organizations" :key="org.id" :value="org.id">{{ org.name }}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div class="space-y-2">
          <Label for="expiry">{{ $t('apiKeys.expiration') }}</Label>
          <Input id="expiry" v-model="formData.expires_at" type="datetime-local" />
          <p class="text-xs text-muted-foreground">{{ $t('apiKeys.expirationHint') }}</p>
        </div>
      </div>
    </CrudFormDialog>
```

- [ ] **Step 5: Add the new i18n keys**

In `frontend/src/i18n/locales/en.json`, inside the `apiKeys` block (`974-1005`), replace the closing lines:

```json
    "never": "Never",
    "nameRequired": "Name is required"
  },
```

with:

```json
    "never": "Never",
    "nameRequired": "Name is required",
    "organization": "Organization",
    "organizationPlaceholder": "Select organization",
    "creatingKeyFor": "Creating key for: {org}",
    "superAdminAllOrgs": "Super Admin (All Organizations)",
    "superAdminBadge": "Super Admin"
  },
```

(`superAdminBadge` isn't used until Task 8, but it's added here alongside its siblings to keep the `apiKeys` i18n block edited in one place.)

- [ ] **Step 6: Typecheck**

Run (from `frontend/`): `npm run typecheck`
Expected: exits with no errors.

- [ ] **Step 7: Manual smoke test**

Run the frontend dev server (`npm run dev`) and the backend, log in as a regular (non-super-admin) user, open Settings → API Keys → Create API Key. Confirm the dialog shows a "Creating key for: {your org name}" line and no dropdown. Then log in as a super admin, repeat, and confirm the dropdown appears, pre-selected to the current org, with a "Super Admin (All Organizations)" option at the top.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/views/settings/APIKeysView.vue frontend/src/i18n/locales/en.json
git commit -m "feat(api-keys): show target org in create dialog, add super-admin org picker"
```

---

### Task 8: Frontend — Organization column and Super Admin badge in the list

**Files:**
- Modify: `frontend/src/views/settings/APIKeysView.vue` (script: `columns`, template: new cell slot)

**Interfaces:**
- Consumes: `APIKey.{organization_name?, is_super_admin_key}` (Task 6), `Badge` component (already imported, `frontend/src/views/settings/APIKeysView.vue:8`).
- Produces: nothing new consumed by later tasks.

- [ ] **Step 1: Add the `organization` column**

Replace the `columns` computed (lines 62-69):

```ts
const columns = computed<Column<APIKey>[]>(() => [
  { key: 'name', label: t('apiKeys.name'), sortable: true },
  { key: 'organization', label: t('apiKeys.organization') },
  { key: 'key', label: t('apiKeys.key') },
  { key: 'last_used', label: t('apiKeys.lastUsed'), sortable: true, sortKey: 'last_used_at' },
  { key: 'expires', label: t('apiKeys.expires'), sortable: true, sortKey: 'expires_at' },
  { key: 'status', label: t('apiKeys.status'), sortable: true, sortKey: 'is_active' },
  { key: 'actions', label: t('common.actions'), align: 'right' },
])
```

- [ ] **Step 2: Add the cell template**

In the template, immediately after the `#cell-name` slot (line 164):

```html
<template #cell-name="{ item: key }"><span class="font-medium">{{ key.name }}</span></template>
<template #cell-organization="{ item: key }">
  <Badge v-if="key.is_super_admin_key" variant="outline" class="border-amber-500 text-amber-600">{{ $t('apiKeys.superAdminBadge') }}</Badge>
  <span v-else class="text-sm">{{ key.organization_name }}</span>
</template>
```

- [ ] **Step 3: Typecheck**

Run (from `frontend/`): `npm run typecheck`
Expected: exits with no errors.

- [ ] **Step 4: Manual smoke test**

With the backend from Task 3 running, reload the API Keys list as any user. Confirm an "Organization" column appears showing the org name for normal keys. Log in as a super admin, create a platform-wide key (Task 7), and confirm it appears in the list with an amber "Super Admin" badge instead of an org name.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/views/settings/APIKeysView.vue
git commit -m "feat(api-keys): show organization / super-admin badge column in the list"
```

---

### Task 9: E2E coverage and manual QA checklist

**Files:**
- Modify: `frontend/e2e/tests/settings/api-keys.spec.ts`

**Interfaces:**
- Consumes: `ApiKeysPage` (`frontend/e2e/pages/SettingsPage.ts:392-421`, unchanged), `loginAsAdmin` (`frontend/e2e/helpers/auth.ts:37`, a regular org admin — not a super admin).

The existing e2e fixtures (`TEST_USERS` in `frontend/e2e/helpers/auth.ts`) don't include a super-admin account, and seeding one is out of scope for this fix. This task automates the regular-user path and documents the super-admin path as manual QA.

- [ ] **Step 1: Add the org-label regression test**

Append to the first `test.describe('API Keys Management', ...)` block in `frontend/e2e/tests/settings/api-keys.spec.ts` (after the `should cancel API key creation` test, before its closing `})`):

```ts
  test('should show organization label in create dialog for a regular admin', async () => {
    await apiKeysPage.openCreateDialog()
    await expect(apiKeysPage.dialog).toContainText('Creating key for:')
  })

  test('should show Organization column in the table', async () => {
    await expect(apiKeysPage.table).toContainText('Organization')
  })
```

- [ ] **Step 2: Run the updated spec**

Run (from `frontend/`, with the backend and frontend dev servers running): `npx playwright test settings/api-keys.spec.ts`
Expected: PASS — all tests in the file pass, including the two new ones.

- [ ] **Step 3: Commit**

```bash
git add frontend/e2e/tests/settings/api-keys.spec.ts
git commit -m "test(e2e): cover organization label and column on the API keys page"
```

- [ ] **Step 4: Manual QA checklist (super admin — no automated fixture exists yet)**

Document and walk through by hand, logged in as a super admin:
1. Create dialog shows the org dropdown (not the read-only label) with "Super Admin (All Organizations)" as the first option, and the current org pre-selected.
2. Selecting a different org and creating a key stores it under that org (verify via the list's Organization column).
3. Selecting "Super Admin (All Organizations)" and creating a key shows the amber "Super Admin" badge in the list instead of an org name.
4. The list shows keys from multiple organizations at once (not just the super admin's own org).
5. A super admin can delete a key belonging to a different organization, and a platform-wide key.
6. A regular (non-super-admin) user still only ever sees/creates/deletes keys in their own organization, with no dropdown — only the read-only "Creating key for: {org}" label.

---

## Self-Review Notes

- **Spec coverage:** org display for regular users → Task 7; super-admin org/platform picker → Task 7; platform-wide key creation and cross-org access → Tasks 1-2; list showing org/badge → Tasks 3, 8; super admin sees all orgs' keys → Task 3; delete across orgs → Task 4; middleware org-context correctness → Tasks 1, 5. All spec sections are covered.
- **Placeholder scan:** no TBD/TODO markers; every step has literal code or an exact command with expected output.
- **Type consistency:** `APIKeyRequest.OrganizationID *uuid.UUID` / `IsSuperAdminKey bool` (Task 2) match what Task 6's frontend payload sends (`organization_id?: string`, `is_super_admin_key?: boolean` — JSON string UUID unmarshals into `*uuid.UUID` via `google/uuid`'s `UnmarshalText`). `APIKeyResponse.OrganizationName`/`IsSuperAdminKey` (Task 3) match the `APIKey` interface fields consumed in Tasks 6 and 8 (`organization_name?`, `is_super_admin_key`). `models.APIKey.OrganizationID *uuid.UUID` (Task 1) is used consistently as a pointer across Tasks 1-5, with `require.NotNil`/dereference at every read site.
