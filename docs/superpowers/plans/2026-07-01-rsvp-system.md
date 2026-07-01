# RSVP System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a standard, org-scoped RSVP feature to Whatomate where an RSVP Event links a chatbot flow (dynamic questions) to structured, tallied guest responses, with campaign + keyword entry, reminders, cutoff, a live dashboard, and Excel export.

**Architecture:** Thin layer over existing subsystems. A new `RSVPEvent` + `RSVPResponse` model pair stores events and answers. Guests answer through the existing chatbot flow engine; answers already accumulate in `ChatbotSession.SessionData` (JSONB) via each step's `StoreAs`, so RSVP capture happens by mapping that session data into an `RSVPResponse` when the flow completes. Keyword entry reuses `ChatbotFlow.TriggerKeywords`; reminders reuse the background scheduler ticker; export reuses the campaign XLSX workbook pattern. Frontend adds three Vue views + an API service.

**Tech Stack:** Go (Fastglue + GORM + PostgreSQL), Vue 3 + TypeScript + shadcn-vue + Vite, excelize (existing).

## Global Constraints

- Multi-tenant: every query filtered by `organization_id`. Never leak across orgs.
- Dates displayed to users MUST be **dd/mm/yyyy** (e.g. 09/06/2026). Store/transmit ISO; format only at display.
- Follow existing patterns exactly: handlers use `a.getOrgID(r)` / `a.getOrgAndUserID(r)` / `a.decodeRequest(r, &v)` and return `r.SendEnvelope(...)` / `r.SendErrorEnvelope(...)`.
- GORM models embed `models.BaseModel` (provides `ID uuid.UUID`, timestamps) and declare a `TableName()`.
- New permission resource is `rsvp` with actions `read`/`write`/`delete`/`execute`/`export`.
- TDD: write the failing test first, watch it fail, implement minimally, watch it pass, commit. Backend tests require `TEST_REDIS_URL` (tests self-skip without it) and a test Postgres via `testutil.SetupTestDB`.
- Naming is fixed by this plan; do not rename across tasks. Model `RSVPEvent` (table `rsvp_events`), `RSVPResponse` (table `rsvp_responses`). Attendance values: `pending` / `yes` / `no` / `maybe`.

---

## File Structure

**Backend (create):**
- `internal/models/rsvp.go` — `RSVPEvent`, `RSVPResponse`, status/attendance constants.
- `internal/handlers/rsvp.go` — event + response CRUD, tally, activate/close, invite send, XLSX export.
- `internal/handlers/rsvp_capture.go` — `finalizeRSVPFromSession`, `seedPendingRSVPResponse`.
- `internal/handlers/rsvp_scheduler.go` — `StartRSVPReminderProcessor`, `ProcessDueRSVPReminders`.
- `internal/handlers/rsvp_test.go`, `internal/handlers/rsvp_capture_test.go`, `internal/handlers/rsvp_scheduler_test.go`.

**Backend (modify):**
- `internal/database/postgres.go` — register the two models in `GetMigrationModels()`.
- `internal/models/roles.go` — `ResourceRSVP` constant + default/system-role permissions.
- `cmd/whatomate/main.go` — register routes + start the reminder ticker.
- `internal/handlers/chatbot_processor.go` — call capture/seed hooks at flow start + completion.

**Frontend (create):**
- `frontend/src/views/rsvp/RSVPEventsView.vue`, `RSVPEventBuilderView.vue`, `RSVPResultsView.vue`.

**Frontend (modify):**
- `frontend/src/services/api.ts` — `rsvpService`.
- `frontend/src/router/index.ts` — three routes.
- `frontend/src/components/layout/navigation.ts` — nav entry.
- `frontend/src/lib/utils.ts` — `formatDateDDMMYYYY` helper.
- `frontend/src/i18n/` locale files — `rsvp.*` and `nav.rsvp` keys.

---

## Task 1: Data model + migration

**Files:**
- Create: `internal/models/rsvp.go`
- Modify: `internal/database/postgres.go` (add to `GetMigrationModels()`)
- Test: `internal/handlers/rsvp_test.go`

**Interfaces:**
- Produces: `models.RSVPEvent`, `models.RSVPResponse`, constants `RSVPEventStatusDraft/Active/Closed`, `RSVPAttendancePending/Yes/No/Maybe`.

- [ ] **Step 1: Write the failing test** — append to `internal/handlers/rsvp_test.go`:

```go
package handlers_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"whatomate/internal/models" // adjust to the module's real import path used by sibling tests
	"whatomate/internal/testutil"
)

func TestRSVPModels_Migrate_And_CRUD(t *testing.T) {
	db := testutil.SetupTestDB(t)
	org := testutil.CreateTestOrganization(t, db)

	event := models.RSVPEvent{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		Name:            "Annual Gala",
		Status:          models.RSVPEventStatusDraft,
		Keyword:         "GALA",
		AttendanceField: "attendance",
		AttendanceMap:   models.JSONB{"yes": "yes", "no": "no", "maybe": "maybe"},
	}
	require.NoError(t, db.Create(&event).Error)

	resp := models.RSVPResponse{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		RSVPEventID:    event.ID,
		OrganizationID: org.ID,
		ContactID:      uuid.New(),
		PhoneNumber:    "15551230000",
		Attendance:     models.RSVPAttendancePending,
		Answers:        models.JSONB{},
	}
	require.NoError(t, db.Create(&resp).Error)

	var got models.RSVPResponse
	require.NoError(t, db.First(&got, "id = ?", resp.ID).Error)
	require.Equal(t, models.RSVPAttendancePending, got.Attendance)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/handlers/ -run TestRSVPModels_Migrate_And_CRUD -v`
Expected: FAIL — `undefined: models.RSVPEvent` (compile error).

- [ ] **Step 3: Create the model** — `internal/models/rsvp.go`:

```go
package models

import (
	"time"

	"github.com/google/uuid"
)

type RSVPEventStatus string

const (
	RSVPEventStatusDraft  RSVPEventStatus = "draft"
	RSVPEventStatusActive RSVPEventStatus = "active"
	RSVPEventStatusClosed RSVPEventStatus = "closed"
)

type RSVPAttendance string

const (
	RSVPAttendancePending RSVPAttendance = "pending"
	RSVPAttendanceYes     RSVPAttendance = "yes"
	RSVPAttendanceNo      RSVPAttendance = "no"
	RSVPAttendanceMaybe   RSVPAttendance = "maybe"
)

// RSVPEvent is an org-scoped RSVP that links a chatbot flow to tallied responses.
type RSVPEvent struct {
	BaseModel
	OrganizationID uuid.UUID       `gorm:"type:uuid;index;not null" json:"organization_id"`
	Name           string          `gorm:"size:255;not null" json:"name"`
	Description    string          `gorm:"type:text" json:"description"`
	EventDate      *time.Time      `json:"event_date,omitempty"`
	RSVPCloseAt    *time.Time      `json:"rsvp_close_at,omitempty"`
	Status         RSVPEventStatus `gorm:"size:20;default:'draft'" json:"status"`

	// WhatsApp account (by Name) used to send invites/reminders.
	WhatsAppAccount string `gorm:"size:100;index" json:"whatsapp_account"`

	// Entry: linked flow + keyword. Keyword must be unique among active events per org.
	FlowID  *uuid.UUID `gorm:"type:uuid" json:"flow_id,omitempty"`
	Keyword string     `gorm:"size:100;index" json:"keyword"`

	// Attendance mapping: which SessionData key holds the attendance answer,
	// and how its raw value maps to yes/no/maybe.
	AttendanceField string `gorm:"size:100;default:'attendance'" json:"attendance_field"`
	AttendanceMap   JSONB  `gorm:"type:jsonb;default:'{}'" json:"attendance_map"`

	// Invite template (optional, for campaign/keyword invite send).
	TemplateID *uuid.UUID `gorm:"type:uuid" json:"template_id,omitempty"`

	// Reminders.
	ReminderEnabled    bool       `gorm:"default:false" json:"reminder_enabled"`
	ReminderAt         *time.Time `json:"reminder_at,omitempty"`
	ReminderTemplateID *uuid.UUID `gorm:"type:uuid" json:"reminder_template_id,omitempty"`
	ReminderSentAt     *time.Time `json:"reminder_sent_at,omitempty"`

	CreatedBy uuid.UUID `gorm:"type:uuid;not null" json:"created_by"`

	Organization *Organization  `gorm:"foreignKey:OrganizationID" json:"organization,omitempty"`
	Responses    []RSVPResponse `gorm:"foreignKey:RSVPEventID" json:"responses,omitempty"`
}

func (RSVPEvent) TableName() string { return "rsvp_events" }

// RSVPResponse is one guest's answer set for an event. Unique per (event, contact).
type RSVPResponse struct {
	BaseModel
	RSVPEventID    uuid.UUID      `gorm:"type:uuid;index:idx_rsvp_event_contact,unique;not null" json:"rsvp_event_id"`
	OrganizationID uuid.UUID      `gorm:"type:uuid;index;not null" json:"organization_id"`
	ContactID      uuid.UUID      `gorm:"type:uuid;index:idx_rsvp_event_contact,unique;not null" json:"contact_id"`
	PhoneNumber    string         `gorm:"size:50;not null" json:"phone_number"`
	Attendance     RSVPAttendance `gorm:"size:20;default:'pending'" json:"attendance"`
	Answers        JSONB          `gorm:"type:jsonb;default:'{}'" json:"answers"`
	RespondedAt    *time.Time     `json:"responded_at,omitempty"`

	Event   *RSVPEvent `gorm:"foreignKey:RSVPEventID" json:"event,omitempty"`
	Contact *Contact   `gorm:"foreignKey:ContactID" json:"contact,omitempty"`
}

func (RSVPResponse) TableName() string { return "rsvp_responses" }
```

- [ ] **Step 4: Register migration** — in `internal/database/postgres.go`, inside `GetMigrationModels()`, add after the `BulkMessageRecipient` entry:

```go
		{"RSVPEvent", &models.RSVPEvent{}},
		{"RSVPResponse", &models.RSVPResponse{}},
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/handlers/ -run TestRSVPModels_Migrate_And_CRUD -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/models/rsvp.go internal/database/postgres.go internal/handlers/rsvp_test.go
git commit -m "feat(rsvp): add RSVPEvent/RSVPResponse models and migration"
```

---

## Task 2: Permission resource

**Files:**
- Modify: `internal/models/roles.go`
- Test: `internal/handlers/rsvp_test.go`

**Interfaces:**
- Produces: `models.ResourceRSVP = "rsvp"`; RSVP entries in `DefaultPermissions()` and `SystemRolePermissions()`.

- [ ] **Step 1: Write the failing test** — append to `internal/handlers/rsvp_test.go`:

```go
func TestRSVPPermissions_InDefaults(t *testing.T) {
	perms := models.DefaultPermissions()
	found := 0
	for _, p := range perms {
		if p.Resource == models.ResourceRSVP {
			found++
		}
	}
	require.GreaterOrEqual(t, found, 4) // read, write, delete, execute (+export)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/handlers/ -run TestRSVPPermissions_InDefaults -v`
Expected: FAIL — `undefined: models.ResourceRSVP`.

- [ ] **Step 3: Add the resource + permissions** — in `internal/models/roles.go`:

Add the constant next to `ResourceCampaigns`:

```go
	ResourceRSVP = "rsvp"
```

Add to `DefaultPermissions()` after the Campaigns block:

```go
		// RSVP
		{Resource: ResourceRSVP, Action: ActionRead, Description: "View RSVP events"},
		{Resource: ResourceRSVP, Action: ActionWrite, Description: "Create and edit RSVP events"},
		{Resource: ResourceRSVP, Action: ActionDelete, Description: "Delete RSVP events"},
		{Resource: ResourceRSVP, Action: ActionExecute, Description: "Send invites/reminders and manage responses"},
		{Resource: ResourceRSVP, Action: ActionExport, Description: "Export RSVP responses"},
```

In `SystemRolePermissions()`, add to the manager permission list:

```go
		"rsvp:read", "rsvp:write", "rsvp:delete", "rsvp:execute", "rsvp:export",
```

and to the agent permission list:

```go
		"rsvp:read",
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/handlers/ -run TestRSVPPermissions_InDefaults -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/models/roles.go internal/handlers/rsvp_test.go
git commit -m "feat(rsvp): add rsvp permission resource and role grants"
```

---

## Task 3: Event CRUD handlers + routes

**Files:**
- Create: `internal/handlers/rsvp.go`
- Modify: `cmd/whatomate/main.go` (routes)
- Test: `internal/handlers/rsvp_test.go`

**Interfaces:**
- Produces handlers: `ListRSVPEvents`, `CreateRSVPEvent`, `GetRSVPEvent`, `UpdateRSVPEvent`, `DeleteRSVPEvent`.
- Produces request type `rsvpEventRequest`, helper `applyRSVPEventRequest`, `parseOptionalUUID`.
- Consumes existing helpers: `a.getOrgID`, `a.getOrgAndUserID`, `a.decodeRequest`, `parsePagination`, `findByIDAndOrg[T]`, `r.SendEnvelope`, `r.SendErrorEnvelope`.

- [ ] **Step 1: Write the failing test** — append to `internal/handlers/rsvp_test.go`:

```go
func TestApp_CreateAndListRSVPEvent(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	user := testutil.CreateTestUser(t, app.DB, org.ID)

	body := `{"name":"Gala","description":"Yearly","keyword":"GALA"}`
	req := testutil.NewPOSTRequest(t, []byte(body))
	testutil.SetAuthContext(req, org.ID, user.ID)
	require.NoError(t, app.CreateRSVPEvent(req))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	lreq := testutil.NewGETRequest(t)
	testutil.SetAuthContext(lreq, org.ID, user.ID)
	require.NoError(t, app.ListRSVPEvents(lreq))

	var resp struct {
		Data struct {
			Events []map[string]any `json:"events"`
			Total  int              `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(testutil.GetResponseBody(lreq), &resp))
	require.Equal(t, 1, resp.Data.Total)
	require.Equal(t, "Gala", resp.Data.Events[0]["name"])
}
```

> Note: if `testutil.NewPOSTRequest` has a different signature in this repo, mirror the exact call used in `campaigns_test.go`'s create test. Do not invent one.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/handlers/ -run TestApp_CreateAndListRSVPEvent -v`
Expected: FAIL — `undefined: app.CreateRSVPEvent`.

- [ ] **Step 3: Implement handlers** — `internal/handlers/rsvp.go`:

```go
package handlers

import (
	"time"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"whatomate/internal/models" // adjust import path to match sibling handlers
)

type rsvpEventRequest struct {
	Name               string       `json:"name"`
	Description        string       `json:"description"`
	EventDate          *time.Time   `json:"event_date"`
	RSVPCloseAt        *time.Time   `json:"rsvp_close_at"`
	WhatsAppAccount    string       `json:"whatsapp_account"`
	FlowID             *string      `json:"flow_id"`
	Keyword            string       `json:"keyword"`
	AttendanceField    string       `json:"attendance_field"`
	AttendanceMap      models.JSONB `json:"attendance_map"`
	TemplateID         *string      `json:"template_id"`
	ReminderEnabled    bool         `json:"reminder_enabled"`
	ReminderAt         *time.Time   `json:"reminder_at"`
	ReminderTemplateID *string      `json:"reminder_template_id"`
}

func parseOptionalUUID(s *string) (*uuid.UUID, bool) {
	if s == nil || *s == "" {
		return nil, true
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return nil, false
	}
	return &id, true
}

func (a *App) applyRSVPEventRequest(e *models.RSVPEvent, req rsvpEventRequest) bool {
	e.Name = req.Name
	e.Description = req.Description
	e.EventDate = req.EventDate
	e.RSVPCloseAt = req.RSVPCloseAt
	e.WhatsAppAccount = req.WhatsAppAccount
	e.Keyword = req.Keyword
	if req.AttendanceField != "" {
		e.AttendanceField = req.AttendanceField
	} else if e.AttendanceField == "" {
		e.AttendanceField = "attendance"
	}
	if req.AttendanceMap != nil {
		e.AttendanceMap = req.AttendanceMap
	}
	e.ReminderEnabled = req.ReminderEnabled
	e.ReminderAt = req.ReminderAt
	if fid, ok := parseOptionalUUID(req.FlowID); ok {
		e.FlowID = fid
	} else {
		return false
	}
	if tid, ok := parseOptionalUUID(req.TemplateID); ok {
		e.TemplateID = tid
	} else {
		return false
	}
	if rid, ok := parseOptionalUUID(req.ReminderTemplateID); ok {
		e.ReminderTemplateID = rid
	} else {
		return false
	}
	return true
}

func (a *App) ListRSVPEvents(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	pg := parsePagination(r)
	search := string(r.RequestCtx.QueryArgs().Peek("search"))
	status := string(r.RequestCtx.QueryArgs().Peek("status"))

	q := a.DB.Model(&models.RSVPEvent{}).Where("organization_id = ?", orgID)
	if search != "" {
		q = q.Where("name ILIKE ?", "%"+search+"%")
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	q.Count(&total)

	var events []models.RSVPEvent
	if err := pg.Apply(q.Order("created_at DESC")).Find(&events).Error; err != nil {
		a.Log.Error("Failed to list rsvp events", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to list RSVP events", nil, "")
	}
	return r.SendEnvelope(map[string]interface{}{
		"events": events,
		"total":  total,
		"page":   pg.Page,
		"limit":  pg.Limit,
	})
}

func (a *App) CreateRSVPEvent(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	var req rsvpEventRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	if req.Name == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Name is required", nil, "")
	}
	event := models.RSVPEvent{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  orgID,
		Status:          models.RSVPEventStatusDraft,
		AttendanceField: "attendance",
		CreatedBy:       userID,
	}
	if !a.applyRSVPEventRequest(&event, req) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid UUID field", nil, "")
	}
	if err := a.DB.Create(&event).Error; err != nil {
		a.Log.Error("Failed to create rsvp event", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to create RSVP event", nil, "")
	}
	return r.SendEnvelope(map[string]interface{}{"id": event.ID, "name": event.Name})
}

func (a *App) GetRSVPEvent(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	id, err := uuid.Parse(r.RequestCtx.UserValue("id").(string))
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid id", nil, "")
	}
	event, err := findByIDAndOrg[models.RSVPEvent](a.DB, r, id, orgID, "RSVP event")
	if err != nil {
		return nil
	}
	return r.SendEnvelope(event)
}

func (a *App) UpdateRSVPEvent(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	id, err := uuid.Parse(r.RequestCtx.UserValue("id").(string))
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid id", nil, "")
	}
	event, err := findByIDAndOrg[models.RSVPEvent](a.DB, r, id, orgID, "RSVP event")
	if err != nil {
		return nil
	}
	var req rsvpEventRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	if !a.applyRSVPEventRequest(event, req) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid UUID field", nil, "")
	}
	if err := a.DB.Save(event).Error; err != nil {
		a.Log.Error("Failed to update rsvp event", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to update RSVP event", nil, "")
	}
	return r.SendEnvelope(map[string]interface{}{"id": event.ID})
}

func (a *App) DeleteRSVPEvent(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	id, err := uuid.Parse(r.RequestCtx.UserValue("id").(string))
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid id", nil, "")
	}
	if err := a.DB.Where("id = ? AND organization_id = ?", id, orgID).
		Delete(&models.RSVPEvent{}).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to delete RSVP event", nil, "")
	}
	return r.SendEnvelope(map[string]interface{}{"deleted": true})
}
```

> If `findByIDAndOrg` / `parsePagination` signatures differ, match their exact use in `campaigns.go`. The path-param accessor `r.RequestCtx.UserValue("id").(string)` must match how `GetCampaign` reads its id — copy that exact idiom.

- [ ] **Step 4: Register routes** — in `cmd/whatomate/main.go`, after the campaign routes:

```go
	// RSVP events
	g.GET("/api/rsvp-events", app.ListRSVPEvents)
	g.POST("/api/rsvp-events", app.CreateRSVPEvent)
	g.GET("/api/rsvp-events/{id}", app.GetRSVPEvent)
	g.PUT("/api/rsvp-events/{id}", app.UpdateRSVPEvent)
	g.DELETE("/api/rsvp-events/{id}", app.DeleteRSVPEvent)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/handlers/ -run TestApp_CreateAndListRSVPEvent -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/handlers/rsvp.go cmd/whatomate/main.go internal/handlers/rsvp_test.go
git commit -m "feat(rsvp): event CRUD handlers and routes"
```

---

## Task 4: Activate/close lifecycle + unique-keyword-per-active-event rule

**Files:**
- Modify: `internal/handlers/rsvp.go` (add `ActivateRSVPEvent`, `CloseRSVPEvent`, `validateUniqueActiveKeyword`, `syncRSVPFlowKeyword` stub)
- Modify: `cmd/whatomate/main.go` (2 routes)
- Test: `internal/handlers/rsvp_test.go`

**Interfaces:**
- Produces: `ActivateRSVPEvent`, `CloseRSVPEvent`, `validateUniqueActiveKeyword(orgID, keyword, excludeID) error`, exported test shim `ValidateUniqueActiveKeywordForTest`, and a compile-only stub `syncRSVPFlowKeyword` (real body added in Task 6).

- [ ] **Step 1: Write the failing test:**

```go
func TestRSVP_UniqueActiveKeyword(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)

	e1 := models.RSVPEvent{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org.ID,
		Name: "A", Keyword: "GALA", Status: models.RSVPEventStatusActive}
	require.NoError(t, app.DB.Create(&e1).Error)

	// Second active event, same keyword -> rejected.
	err := app.ValidateUniqueActiveKeywordForTest(org.ID, "GALA", uuid.New())
	require.Error(t, err)

	// Different keyword -> ok.
	require.NoError(t, app.ValidateUniqueActiveKeywordForTest(org.ID, "OTHER", uuid.New()))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/handlers/ -run TestRSVP_UniqueActiveKeyword -v`
Expected: FAIL — `undefined: ...ValidateUniqueActiveKeywordForTest`.

- [ ] **Step 3: Implement** — add to `internal/handlers/rsvp.go`:

```go
type rsvpError struct{ msg string }

func (e *rsvpError) Error() string { return e.msg }

var errKeywordInUse = &rsvpError{"keyword already used by another active event"}

func (a *App) validateUniqueActiveKeyword(orgID uuid.UUID, keyword string, excludeID uuid.UUID) error {
	if keyword == "" {
		return nil
	}
	var count int64
	a.DB.Model(&models.RSVPEvent{}).
		Where("organization_id = ? AND LOWER(keyword) = LOWER(?) AND status = ? AND id <> ?",
			orgID, keyword, models.RSVPEventStatusActive, excludeID).
		Count(&count)
	if count > 0 {
		return errKeywordInUse
	}
	return nil
}

func (a *App) ValidateUniqueActiveKeywordForTest(orgID uuid.UUID, keyword string, excludeID uuid.UUID) error {
	return a.validateUniqueActiveKeyword(orgID, keyword, excludeID)
}

// syncRSVPFlowKeyword ensures the linked flow's TriggerKeywords include the event keyword.
// Full behavior added in Task 6; stubbed here so the package compiles.
func (a *App) syncRSVPFlowKeyword(event *models.RSVPEvent) {}

func (a *App) ActivateRSVPEvent(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	id, err := uuid.Parse(r.RequestCtx.UserValue("id").(string))
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid id", nil, "")
	}
	event, err := findByIDAndOrg[models.RSVPEvent](a.DB, r, id, orgID, "RSVP event")
	if err != nil {
		return nil
	}
	if verr := a.validateUniqueActiveKeyword(orgID, event.Keyword, event.ID); verr != nil {
		return r.SendErrorEnvelope(fasthttp.StatusConflict, verr.Error(), nil, "")
	}
	event.Status = models.RSVPEventStatusActive
	if err := a.DB.Model(event).Update("status", event.Status).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to activate", nil, "")
	}
	a.syncRSVPFlowKeyword(event)
	return r.SendEnvelope(map[string]interface{}{"id": event.ID, "status": event.Status})
}

func (a *App) CloseRSVPEvent(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	id, err := uuid.Parse(r.RequestCtx.UserValue("id").(string))
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid id", nil, "")
	}
	event, err := findByIDAndOrg[models.RSVPEvent](a.DB, r, id, orgID, "RSVP event")
	if err != nil {
		return nil
	}
	event.Status = models.RSVPEventStatusClosed
	if err := a.DB.Model(event).Update("status", event.Status).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to close", nil, "")
	}
	return r.SendEnvelope(map[string]interface{}{"id": event.ID, "status": event.Status})
}
```

- [ ] **Step 4: Register routes** — in `cmd/whatomate/main.go`:

```go
	g.POST("/api/rsvp-events/{id}/activate", app.ActivateRSVPEvent)
	g.POST("/api/rsvp-events/{id}/close", app.CloseRSVPEvent)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/handlers/ -run TestRSVP_UniqueActiveKeyword -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/handlers/rsvp.go cmd/whatomate/main.go internal/handlers/rsvp_test.go
git commit -m "feat(rsvp): activate/close lifecycle with unique-active-keyword rule"
```

---

## Task 5: Responses list + tally endpoint

**Files:**
- Modify: `internal/handlers/rsvp.go` (`ListRSVPResponses`, `GetRSVPTally`)
- Modify: `cmd/whatomate/main.go` (2 routes)
- Test: `internal/handlers/rsvp_test.go`

**Interfaces:**
- Produces: `ListRSVPResponses`, `GetRSVPTally`. Tally shape: `{"yes":N,"no":N,"maybe":N,"pending":N,"total":N}`.

- [ ] **Step 1: Write the failing test:**

```go
func TestRSVP_Tally(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	user := testutil.CreateTestUser(t, app.DB, org.ID)
	event := models.RSVPEvent{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org.ID, Name: "E"}
	require.NoError(t, app.DB.Create(&event).Error)

	mk := func(att models.RSVPAttendance) {
		require.NoError(t, app.DB.Create(&models.RSVPResponse{
			BaseModel: models.BaseModel{ID: uuid.New()}, RSVPEventID: event.ID,
			OrganizationID: org.ID, ContactID: uuid.New(), PhoneNumber: "1", Attendance: att,
		}).Error)
	}
	mk(models.RSVPAttendanceYes)
	mk(models.RSVPAttendanceYes)
	mk(models.RSVPAttendanceNo)
	mk(models.RSVPAttendancePending)

	req := testutil.NewGETRequest(t)
	testutil.SetAuthContext(req, org.ID, user.ID)
	req.RequestCtx.SetUserValue("id", event.ID.String())
	require.NoError(t, app.GetRSVPTally(req))

	var resp struct {
		Data map[string]int `json:"data"`
	}
	require.NoError(t, json.Unmarshal(testutil.GetResponseBody(req), &resp))
	require.Equal(t, 2, resp.Data["yes"])
	require.Equal(t, 1, resp.Data["no"])
	require.Equal(t, 1, resp.Data["pending"])
	require.Equal(t, 4, resp.Data["total"])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/handlers/ -run TestRSVP_Tally -v`
Expected: FAIL — `undefined: app.GetRSVPTally`.

- [ ] **Step 3: Implement** — add to `internal/handlers/rsvp.go`:

```go
func (a *App) ListRSVPResponses(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	eventID, err := uuid.Parse(r.RequestCtx.UserValue("id").(string))
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid id", nil, "")
	}
	pg := parsePagination(r)
	q := a.DB.Model(&models.RSVPResponse{}).
		Where("organization_id = ? AND rsvp_event_id = ?", orgID, eventID)
	if status := string(r.RequestCtx.QueryArgs().Peek("attendance")); status != "" {
		q = q.Where("attendance = ?", status)
	}
	var total int64
	q.Count(&total)
	var rows []models.RSVPResponse
	if err := pg.Apply(q.Preload("Contact").Order("responded_at DESC NULLS LAST")).Find(&rows).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to list responses", nil, "")
	}
	return r.SendEnvelope(map[string]interface{}{"responses": rows, "total": total, "page": pg.Page, "limit": pg.Limit})
}

func (a *App) GetRSVPTally(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	eventID, err := uuid.Parse(r.RequestCtx.UserValue("id").(string))
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid id", nil, "")
	}
	type row struct {
		Attendance models.RSVPAttendance
		Count      int
	}
	var rows []row
	a.DB.Model(&models.RSVPResponse{}).
		Select("attendance, count(*) as count").
		Where("organization_id = ? AND rsvp_event_id = ?", orgID, eventID).
		Group("attendance").Scan(&rows)

	out := map[string]int{"yes": 0, "no": 0, "maybe": 0, "pending": 0, "total": 0}
	for _, rw := range rows {
		out[string(rw.Attendance)] += rw.Count
		out["total"] += rw.Count
	}
	return r.SendEnvelope(out)
}
```

- [ ] **Step 4: Register routes** — in `cmd/whatomate/main.go`:

```go
	g.GET("/api/rsvp-events/{id}/responses", app.ListRSVPResponses)
	g.GET("/api/rsvp-events/{id}/tally", app.GetRSVPTally)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/handlers/ -run TestRSVP_Tally -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/handlers/rsvp.go cmd/whatomate/main.go internal/handlers/rsvp_test.go
git commit -m "feat(rsvp): responses listing and live tally endpoint"
```

---

## Task 6: Flow capture — seed on start, finalize on completion, keyword sync

**Files:**
- Create: `internal/handlers/rsvp_capture.go`
- Modify: `internal/handlers/rsvp.go` (replace the `syncRSVPFlowKeyword` stub with the real body)
- Modify: `internal/handlers/chatbot_processor.go` (call hooks at flow start + completion + cutoff check)
- Test: `internal/handlers/rsvp_capture_test.go`

**Interfaces:**
- Produces: `seedPendingRSVPResponse(orgID uuid.UUID, event *models.RSVPEvent, contactID uuid.UUID, phone string)`, `finalizeRSVPFromSession(session *models.ChatbotSession)`, `rsvpEventForFlow(orgID, flowID uuid.UUID) *models.RSVPEvent`, `mapAttendance`, const `rsvpEventIDKey`, test shim `FinalizeRSVPFromSessionForTest`.
- Consumes: `models.ChatbotSession.SessionData` (JSONB), `models.ChatbotFlow.TriggerKeywords`, `contactutil.GetOrCreateContact` (already called in the processor's message-ingest path).

- [ ] **Step 1: Write the failing test** — `internal/handlers/rsvp_capture_test.go`:

```go
package handlers_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"whatomate/internal/models"
	"whatomate/internal/testutil"
)

func TestFinalizeRSVPFromSession_MapsAnswers(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	contactID := uuid.New()

	event := models.RSVPEvent{
		BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org.ID, Name: "Gala",
		Status: models.RSVPEventStatusActive, AttendanceField: "attendance",
		AttendanceMap: models.JSONB{"yes": "yes", "no": "no", "maybe": "maybe"},
	}
	require.NoError(t, app.DB.Create(&event).Error)

	// Pre-seed a pending response (as campaign/keyword entry would).
	require.NoError(t, app.DB.Create(&models.RSVPResponse{
		BaseModel: models.BaseModel{ID: uuid.New()}, RSVPEventID: event.ID, OrganizationID: org.ID,
		ContactID: contactID, PhoneNumber: "15551230000", Attendance: models.RSVPAttendancePending,
		Answers: models.JSONB{},
	}).Error)

	flowID := uuid.New()
	session := &models.ChatbotSession{
		BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org.ID, ContactID: contactID,
		PhoneNumber: "15551230000", CurrentFlowID: &flowID,
		SessionData: models.JSONB{
			"_rsvp_event_id": event.ID.String(),
			"attendance":     "yes",
			"headcount":      "3",
		},
	}

	app.FinalizeRSVPFromSessionForTest(session)

	var got models.RSVPResponse
	require.NoError(t, app.DB.First(&got, "rsvp_event_id = ? AND contact_id = ?", event.ID, contactID).Error)
	require.Equal(t, models.RSVPAttendanceYes, got.Attendance)
	require.Equal(t, "3", got.Answers["headcount"])
	require.NotNil(t, got.RespondedAt)
	require.WithinDuration(t, time.Now(), *got.RespondedAt, time.Minute)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/handlers/ -run TestFinalizeRSVPFromSession_MapsAnswers -v`
Expected: FAIL — `undefined: ...FinalizeRSVPFromSessionForTest`.

- [ ] **Step 3: Implement capture** — `internal/handlers/rsvp_capture.go`:

```go
package handlers

import (
	"time"

	"github.com/google/uuid"
	"whatomate/internal/models"
)

// internal SessionData key used by RSVP capture.
const rsvpEventIDKey = "_rsvp_event_id"

// rsvpEventForFlow returns the active RSVP event linked to a flow, or nil.
func (a *App) rsvpEventForFlow(orgID, flowID uuid.UUID) *models.RSVPEvent {
	var event models.RSVPEvent
	if err := a.DB.Where("organization_id = ? AND flow_id = ? AND status = ?",
		orgID, flowID, models.RSVPEventStatusActive).First(&event).Error; err != nil {
		return nil
	}
	return &event
}

// seedPendingRSVPResponse creates a pending response row for a contact entering an event.
// No-op if a row already exists (does not overwrite an answered row).
func (a *App) seedPendingRSVPResponse(orgID uuid.UUID, event *models.RSVPEvent, contactID uuid.UUID, phone string) {
	var existing models.RSVPResponse
	if err := a.DB.Where("rsvp_event_id = ? AND contact_id = ?", event.ID, contactID).First(&existing).Error; err == nil {
		return
	}
	_ = a.DB.Create(&models.RSVPResponse{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		RSVPEventID:    event.ID,
		OrganizationID: orgID,
		ContactID:      contactID,
		PhoneNumber:    phone,
		Attendance:     models.RSVPAttendancePending,
		Answers:        models.JSONB{},
	}).Error
}

// finalizeRSVPFromSession maps completed-flow SessionData into the RSVPResponse.
// No-op if the session is not tied to an RSVP event.
func (a *App) finalizeRSVPFromSession(session *models.ChatbotSession) {
	if session == nil || session.SessionData == nil {
		return
	}
	raw, ok := session.SessionData[rsvpEventIDKey].(string)
	if !ok || raw == "" {
		return
	}
	eventID, err := uuid.Parse(raw)
	if err != nil {
		return
	}
	var event models.RSVPEvent
	if err := a.DB.Where("id = ? AND organization_id = ?", eventID, session.OrganizationID).
		First(&event).Error; err != nil {
		return
	}

	// Build answers map (exclude internal keys prefixed with '_').
	answers := models.JSONB{}
	for k, v := range session.SessionData {
		if len(k) > 0 && k[0] == '_' {
			continue
		}
		answers[k] = v
	}

	// Derive attendance from the configured field + map.
	attendance := models.RSVPAttendancePending
	if event.AttendanceField != "" {
		if val, ok := session.SessionData[event.AttendanceField]; ok {
			attendance = mapAttendance(event.AttendanceMap, val)
		}
	}

	now := time.Now()
	updates := map[string]interface{}{
		"answers":      answers,
		"attendance":   attendance,
		"responded_at": now,
	}
	// Upsert: update existing (pending) row, else create.
	res := a.DB.Model(&models.RSVPResponse{}).
		Where("rsvp_event_id = ? AND contact_id = ?", event.ID, session.ContactID).
		Updates(updates)
	if res.Error == nil && res.RowsAffected == 0 {
		_ = a.DB.Create(&models.RSVPResponse{
			BaseModel:      models.BaseModel{ID: uuid.New()},
			RSVPEventID:    event.ID,
			OrganizationID: session.OrganizationID,
			ContactID:      session.ContactID,
			PhoneNumber:    session.PhoneNumber,
			Attendance:     attendance,
			Answers:        answers,
			RespondedAt:    &now,
		}).Error
	}
}

func mapAttendance(m models.JSONB, raw interface{}) models.RSVPAttendance {
	s, _ := raw.(string)
	if m != nil {
		if mapped, ok := m[s].(string); ok {
			s = mapped
		}
	}
	switch models.RSVPAttendance(s) {
	case models.RSVPAttendanceYes:
		return models.RSVPAttendanceYes
	case models.RSVPAttendanceNo:
		return models.RSVPAttendanceNo
	case models.RSVPAttendanceMaybe:
		return models.RSVPAttendanceMaybe
	default:
		return models.RSVPAttendancePending
	}
}

func (a *App) FinalizeRSVPFromSessionForTest(s *models.ChatbotSession) { a.finalizeRSVPFromSession(s) }
```

- [ ] **Step 4: Implement keyword sync** — in `internal/handlers/rsvp.go`, replace the `syncRSVPFlowKeyword` stub body with:

```go
func (a *App) syncRSVPFlowKeyword(event *models.RSVPEvent) {
	if event.FlowID == nil || event.Keyword == "" {
		return
	}
	var flow models.ChatbotFlow
	if err := a.DB.Where("id = ? AND organization_id = ?", *event.FlowID, event.OrganizationID).
		First(&flow).Error; err != nil {
		return
	}
	for _, k := range flow.TriggerKeywords {
		if k == event.Keyword {
			return
		}
	}
	flow.TriggerKeywords = append(flow.TriggerKeywords, event.Keyword)
	a.DB.Model(&flow).Update("trigger_keywords", flow.TriggerKeywords)
}
```

> Confirm `ChatbotFlow.TriggerKeywords` is a `StringArray`/`[]string` GORM field (it is, per the flow engine) and the column name is `trigger_keywords`. If the column name differs, use the exact gorm column from `internal/models/chatbot.go`.

- [ ] **Step 5: Wire hooks into the flow processor** — in `internal/handlers/chatbot_processor.go`:

(a) At flow start. Find `startFlow(...)` and, right after the session's `CurrentFlowID` is set and persisted, add:

```go
	// RSVP: if this flow belongs to an active RSVP event, tag the session and seed a pending row.
	if event := a.rsvpEventForFlow(account.OrganizationID, flow.ID); event != nil {
		// Cutoff: refuse if the RSVP has closed.
		if event.RSVPCloseAt != nil && time.Now().After(*event.RSVPCloseAt) {
			a.sendAndSaveTextMessage(account, contact, "Sorry, RSVP for this event is now closed.")
			a.exitFlow(session)
			return
		}
		if session.SessionData == nil {
			session.SessionData = models.JSONB{}
		}
		session.SessionData[rsvpEventIDKey] = event.ID.String()
		a.DB.Model(session).Update("session_data", session.SessionData)
		a.seedPendingRSVPResponse(account.OrganizationID, event, contact.ID, contact.PhoneNumber)
	}
```

> `a.sendAndSaveTextMessage`, `a.exitFlow`, and the `account`/`contact`/`session`/`flow` variable names are those already in scope in `startFlow` (confirmed in the flow engine). If the send-text helper name differs, use the one `startFlow` already uses for greeting/out-of-hours messages.

(b) At flow completion. Find where a flow session is completed (search for `SessionStatusCompleted` and the `exitFlow` call in `processFlowResponse`). Add the capture call at the single point where the flow reaches its natural end:

```go
	a.finalizeRSVPFromSession(session)
```

> If completion is centralized in `exitFlow`, add `a.finalizeRSVPFromSession(session)` as the FIRST line of `exitFlow` instead — it is a safe no-op for non-RSVP sessions and guarantees capture on every completion path. Only add it in ONE place to avoid double-processing (the upsert makes a double call harmless, but keep it single).

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/handlers/ -run TestFinalizeRSVPFromSession_MapsAnswers -v`
Then: `go build ./...`
Expected: test PASS, build succeeds.

- [ ] **Step 7: Commit**

```bash
git add internal/handlers/rsvp_capture.go internal/handlers/rsvp.go internal/handlers/chatbot_processor.go internal/handlers/rsvp_capture_test.go
git commit -m "feat(rsvp): capture flow answers into responses, seed on entry, keyword sync, cutoff"
```

---

## Task 7: Invite send (campaign + keyword entry seeding)

**Files:**
- Modify: `internal/handlers/rsvp.go` (`SendRSVPInvites`, `sendRSVPInviteTemplate`, `sendInvitesRequest`)
- Modify: `cmd/whatomate/main.go` (1 route)
- Test: `internal/handlers/rsvp_test.go`

**Interfaces:**
- Produces: `SendRSVPInvites` — seeds pending responses for a supplied contact-id list and (if a template is configured) sends the invite template per contact.
- Consumes: `a.seedPendingRSVPResponse`, the real single-recipient template-send helper used by the campaign worker (see note; referred to below as `sendApprovedTemplate`).

- [ ] **Step 1: Write the failing test:**

```go
func TestRSVP_SendInvites_SeedsPending(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	user := testutil.CreateTestUser(t, app.DB, org.ID)
	event := models.RSVPEvent{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org.ID,
		Name: "E", Status: models.RSVPEventStatusActive}
	require.NoError(t, app.DB.Create(&event).Error)

	c1 := uuid.New()
	require.NoError(t, app.DB.Create(&models.Contact{
		BaseModel: models.BaseModel{ID: c1}, OrganizationID: org.ID, PhoneNumber: "15551230001",
	}).Error)

	body := `{"contact_ids":["` + c1.String() + `"]}`
	req := testutil.NewPOSTRequest(t, []byte(body))
	testutil.SetAuthContext(req, org.ID, user.ID)
	req.RequestCtx.SetUserValue("id", event.ID.String())
	require.NoError(t, app.SendRSVPInvites(req))

	var count int64
	app.DB.Model(&models.RSVPResponse{}).
		Where("rsvp_event_id = ? AND attendance = ?", event.ID, models.RSVPAttendancePending).
		Count(&count)
	require.Equal(t, int64(1), count)
}
```

> If `models.Contact`'s required fields differ, mirror `testutil.CreateTestContact` if one exists, or the minimal fields used elsewhere. Do not invent columns.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/handlers/ -run TestRSVP_SendInvites_SeedsPending -v`
Expected: FAIL — `undefined: app.SendRSVPInvites`.

- [ ] **Step 3: Implement** — add to `internal/handlers/rsvp.go`:

```go
type sendInvitesRequest struct {
	ContactIDs []string `json:"contact_ids"`
}

func (a *App) SendRSVPInvites(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	eventID, err := uuid.Parse(r.RequestCtx.UserValue("id").(string))
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid id", nil, "")
	}
	event, err := findByIDAndOrg[models.RSVPEvent](a.DB, r, eventID, orgID, "RSVP event")
	if err != nil {
		return nil
	}
	var req sendInvitesRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	seeded := 0
	for _, cidStr := range req.ContactIDs {
		cid, perr := uuid.Parse(cidStr)
		if perr != nil {
			continue
		}
		var contact models.Contact
		if err := a.DB.Where("id = ? AND organization_id = ?", cid, orgID).First(&contact).Error; err != nil {
			continue
		}
		a.seedPendingRSVPResponse(orgID, event, contact.ID, contact.PhoneNumber)
		seeded++
		// Sending is best-effort: a send failure must NOT fail seeding.
		if event.TemplateID != nil && event.WhatsAppAccount != "" {
			a.sendRSVPInviteTemplate(event, &contact)
		}
	}
	return r.SendEnvelope(map[string]interface{}{"seeded": seeded})
}

// sendRSVPInviteTemplate sends the configured invite template to one contact.
func (a *App) sendRSVPInviteTemplate(event *models.RSVPEvent, contact *models.Contact) {
	var account models.WhatsAppAccount
	if err := a.DB.Where("organization_id = ? AND name = ?", event.OrganizationID, event.WhatsAppAccount).
		First(&account).Error; err != nil {
		a.Log.Error("RSVP invite: account not found", "account", event.WhatsAppAccount, "error", err)
		return
	}
	var template models.Template
	if err := a.DB.Where("id = ? AND organization_id = ?", *event.TemplateID, event.OrganizationID).
		First(&template).Error; err != nil {
		a.Log.Error("RSVP invite: template not found", "error", err)
		return
	}
	// Reuse the existing single-recipient template send used by the campaign worker.
	if err := a.sendApprovedTemplate(&account, contact, &template, nil); err != nil {
		a.Log.Error("RSVP invite send failed", "error", err)
	}
}
```

> `sendApprovedTemplate` is a placeholder for the real single-recipient template-send function used by the campaign recipient worker (the code enqueued by `enqueueCampaignRecipients`). During implementation: grep that worker for the actual send call + signature and use it here (and identically in Task 8). Seeding must succeed even if sending is unavailable. Do NOT invent a new send path.

- [ ] **Step 4: Register route** — in `cmd/whatomate/main.go`:

```go
	g.POST("/api/rsvp-events/{id}/send-invites", app.SendRSVPInvites)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/handlers/ -run TestRSVP_SendInvites_SeedsPending -v`
Expected: PASS. (The test supplies no `TemplateID`, so the send path is skipped and only seeding is asserted.)

- [ ] **Step 6: Commit**

```bash
git add internal/handlers/rsvp.go cmd/whatomate/main.go internal/handlers/rsvp_test.go
git commit -m "feat(rsvp): send invites - seed pending responses and enqueue template"
```

---

## Task 8: Reminder scheduler

**Files:**
- Create: `internal/handlers/rsvp_scheduler.go`
- Modify: `internal/handlers/app.go` (add `RSVPReminderCancel context.CancelFunc` field to the `App` struct — put it next to `ScheduledCampaignCancel`)
- Modify: `cmd/whatomate/main.go` (start ticker at boot)
- Test: `internal/handlers/rsvp_scheduler_test.go`

**Interfaces:**
- Produces: `StartRSVPReminderProcessor(interval time.Duration)`, `ProcessDueRSVPReminders(ctx context.Context)`, `dueRSVPReminderEvents() []models.RSVPEvent`, test shim `DueRSVPReminderEventsForTest`.
- Consumes: `a.wg` (existing WaitGroup on `App`), `sendApprovedTemplate` (same helper as Task 7).

- [ ] **Step 1: Write the failing test** — `internal/handlers/rsvp_scheduler_test.go`:

```go
package handlers_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"whatomate/internal/models"
	"whatomate/internal/testutil"
)

func TestProcessDueRSVPReminders_SelectsOnlyDue(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	due := models.RSVPEvent{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org.ID,
		Name: "due", Status: models.RSVPEventStatusActive, ReminderEnabled: true,
		ReminderAt: &past, RSVPCloseAt: &future}
	require.NoError(t, app.DB.Create(&due).Error)

	notDue := models.RSVPEvent{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org.ID,
		Name: "notdue", Status: models.RSVPEventStatusActive, ReminderEnabled: true,
		ReminderAt: &future, RSVPCloseAt: &future}
	require.NoError(t, app.DB.Create(&notDue).Error)

	events := app.DueRSVPReminderEventsForTest()
	require.Len(t, events, 1)
	require.Equal(t, "due", events[0].Name)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/handlers/ -run TestProcessDueRSVPReminders_SelectsOnlyDue -v`
Expected: FAIL — `undefined: ...DueRSVPReminderEventsForTest`.

- [ ] **Step 3: Implement** — `internal/handlers/rsvp_scheduler.go`:

```go
package handlers

import (
	"context"
	"time"

	"whatomate/internal/models"
)

func (a *App) dueRSVPReminderEvents() []models.RSVPEvent {
	now := time.Now()
	var events []models.RSVPEvent
	a.DB.Where(`status = ? AND reminder_enabled = ? AND reminder_at IS NOT NULL
		AND reminder_at <= ? AND reminder_sent_at IS NULL
		AND (rsvp_close_at IS NULL OR rsvp_close_at > ?)`,
		models.RSVPEventStatusActive, true, now, now).
		Find(&events)
	return events
}

// ProcessDueRSVPReminders re-sends invites to pending guests for due events.
func (a *App) ProcessDueRSVPReminders(ctx context.Context) {
	events := a.dueRSVPReminderEvents()
	for i := range events {
		event := &events[i]
		var pending []models.RSVPResponse
		a.DB.Where("rsvp_event_id = ? AND attendance = ?", event.ID, models.RSVPAttendancePending).
			Find(&pending)

		for j := range pending {
			p := &pending[j]
			if event.ReminderTemplateID == nil || event.WhatsAppAccount == "" {
				continue
			}
			var account models.WhatsAppAccount
			if err := a.DB.Where("organization_id = ? AND name = ?", event.OrganizationID, event.WhatsAppAccount).
				First(&account).Error; err != nil {
				continue
			}
			var contact models.Contact
			if err := a.DB.Where("id = ?", p.ContactID).First(&contact).Error; err != nil {
				continue
			}
			var template models.Template
			if err := a.DB.Where("id = ? AND organization_id = ?", *event.ReminderTemplateID, event.OrganizationID).
				First(&template).Error; err != nil {
				continue
			}
			// Same single-recipient template send as invites (Task 7).
			_ = a.sendApprovedTemplate(&account, &contact, &template, nil)
		}
		now := time.Now()
		a.DB.Model(event).Update("reminder_sent_at", now)
	}
}

// StartRSVPReminderProcessor runs ProcessDueRSVPReminders on a ticker, mirroring
// StartScheduledCampaignProcessor.
func (a *App) StartRSVPReminderProcessor(interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.RSVPReminderCancel = cancel

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		a.ProcessDueRSVPReminders(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.ProcessDueRSVPReminders(ctx)
			}
		}
	}()
}

func (a *App) DueRSVPReminderEventsForTest() []models.RSVPEvent { return a.dueRSVPReminderEvents() }
```

> Reuse `sendApprovedTemplate` — the same real send helper resolved in Task 7. Keep the name identical across Tasks 7 and 8. If `a.wg` is unexported and lives in the same package (it does — `handlers`), this compiles; the test only exercises `dueRSVPReminderEvents`.

- [ ] **Step 4: Start ticker at boot** — in `cmd/whatomate/main.go`, next to `app.StartScheduledCampaignProcessor(time.Minute)`:

```go
	app.StartRSVPReminderProcessor(time.Minute)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/handlers/ -run TestProcessDueRSVPReminders_SelectsOnlyDue -v`
Then: `go build ./...`
Expected: test PASS, build succeeds.

- [ ] **Step 6: Commit**

```bash
git add internal/handlers/rsvp_scheduler.go internal/handlers/app.go cmd/whatomate/main.go internal/handlers/rsvp_scheduler_test.go
git commit -m "feat(rsvp): reminder ticker for pending guests before cutoff"
```

---

## Task 9: XLSX export

**Files:**
- Modify: `internal/handlers/rsvp.go` (`ExportRSVPResponses`; add `"sort"` + excelize imports)
- Modify: `cmd/whatomate/main.go` (1 route)
- Test: `internal/handlers/rsvp_test.go`

**Interfaces:**
- Produces: `ExportRSVPResponses` — streams an `.xlsx` of the guest list + answers.
- Consumes: the excelize usage + import alias from `internal/handlers/campaign_report_workbook.go`.

- [ ] **Step 1: Write the failing test:**

```go
func TestRSVP_Export_ReturnsXLSX(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	user := testutil.CreateTestUser(t, app.DB, org.ID)
	event := models.RSVPEvent{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org.ID, Name: "E"}
	require.NoError(t, app.DB.Create(&event).Error)
	require.NoError(t, app.DB.Create(&models.RSVPResponse{
		BaseModel: models.BaseModel{ID: uuid.New()}, RSVPEventID: event.ID, OrganizationID: org.ID,
		ContactID: uuid.New(), PhoneNumber: "15551230000", Attendance: models.RSVPAttendanceYes,
		Answers: models.JSONB{"headcount": "2"},
	}).Error)

	req := testutil.NewGETRequest(t)
	testutil.SetAuthContext(req, org.ID, user.ID)
	req.RequestCtx.SetUserValue("id", event.ID.String())
	require.NoError(t, app.ExportRSVPResponses(req))

	body := testutil.GetResponseBody(req)
	require.GreaterOrEqual(t, len(body), 4)
	// XLSX is a zip; first two bytes are "PK".
	require.Equal(t, byte('P'), body[0])
	require.Equal(t, byte('K'), body[1])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/handlers/ -run TestRSVP_Export_ReturnsXLSX -v`
Expected: FAIL — `undefined: app.ExportRSVPResponses`.

- [ ] **Step 3: Implement** — add to `internal/handlers/rsvp.go` (use the SAME excelize import alias as `campaign_report_workbook.go`, e.g. `"github.com/xuri/excelize/v2"`, and add `"sort"`):

```go
func (a *App) ExportRSVPResponses(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}
	eventID, err := uuid.Parse(r.RequestCtx.UserValue("id").(string))
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid id", nil, "")
	}
	var event models.RSVPEvent
	if err := a.DB.Where("id = ? AND organization_id = ?", eventID, orgID).First(&event).Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, "RSVP event not found", nil, "")
	}
	var rows []models.RSVPResponse
	a.DB.Where("organization_id = ? AND rsvp_event_id = ?", orgID, eventID).
		Preload("Contact").Order("responded_at DESC NULLS LAST").Find(&rows)

	// Collect the union of dynamic answer keys for stable columns.
	keySet := map[string]struct{}{}
	for _, row := range rows {
		for k := range row.Answers {
			keySet[k] = struct{}{}
		}
	}
	answerKeys := make([]string, 0, len(keySet))
	for k := range keySet {
		answerKeys = append(answerKeys, k)
	}
	sort.Strings(answerKeys)

	f := excelize.NewFile()
	sheet := "Responses"
	f.SetSheetName(f.GetSheetName(0), sheet)

	headers := append([]string{"Phone", "Attendance", "Responded At"}, answerKeys...)
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}
	for rIdx, row := range rows {
		respondedAt := ""
		if row.RespondedAt != nil {
			respondedAt = row.RespondedAt.Format("02/01/2006") // dd/mm/yyyy
		}
		vals := []interface{}{row.PhoneNumber, string(row.Attendance), respondedAt}
		for _, k := range answerKeys {
			vals = append(vals, row.Answers[k])
		}
		for cIdx, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(cIdx+1, rIdx+2)
			f.SetCellValue(sheet, cell, v)
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to build workbook", nil, "")
	}
	r.RequestCtx.Response.Header.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	r.RequestCtx.Response.Header.Set("Content-Disposition", `attachment; filename="rsvp-responses.xlsx"`)
	r.RequestCtx.SetBody(buf.Bytes())
	return nil
}
```

- [ ] **Step 4: Register route** — in `cmd/whatomate/main.go`:

```go
	g.GET("/api/rsvp-events/{id}/export", app.ExportRSVPResponses)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/handlers/ -run TestRSVP_Export_ReturnsXLSX -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/handlers/rsvp.go cmd/whatomate/main.go internal/handlers/rsvp_test.go
git commit -m "feat(rsvp): XLSX export of guest list and answers (dd/mm/yyyy)"
```

---

## Task 10: Full backend verification

**Files:** none created; verification only.

- [ ] **Step 1: Run the full backend test suite**

Run: `go test ./... 2>&1 | tail -40`
Expected: PASS (RSVP tests self-skip only if `TEST_REDIS_URL` is unset — if so, set it and re-run to confirm).

- [ ] **Step 2: Vet + build**

Run: `go vet ./... && go build ./...`
Expected: no errors.

- [ ] **Step 3: Commit any fixes**

```bash
git add -A
git commit -m "test(rsvp): backend suite green"
```

---

## Task 11: Frontend API service + date helper

**Files:**
- Modify: `frontend/src/services/api.ts`
- Modify: `frontend/src/lib/utils.ts`

**Interfaces:**
- Produces: `rsvpService` with `list/create/get/update/delete/activate/close/responses/tally/sendInvites/exportUrl`; `formatDateDDMMYYYY(date)`.

- [ ] **Step 1: Add the date helper** — append to `frontend/src/lib/utils.ts`:

```typescript
export function formatDateDDMMYYYY(date: string | Date): string {
  const d = typeof date === 'string' ? new Date(date) : date
  if (isNaN(d.getTime())) return ''
  const day = String(d.getDate()).padStart(2, '0')
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const year = d.getFullYear()
  return `${day}/${month}/${year}`
}
```

- [ ] **Step 2: Add the service** — append to `frontend/src/services/api.ts`:

```typescript
export const rsvpService = {
  list: (params?: { search?: string; status?: string; page?: number; limit?: number }) =>
    api.get("/rsvp-events", { params }),
  create: (data: Record<string, unknown>) =>
    api.post("/rsvp-events", data),
  get: (id: string) =>
    api.get(`/rsvp-events/${id}`),
  update: (id: string, data: Record<string, unknown>) =>
    api.put(`/rsvp-events/${id}`, data),
  delete: (id: string) =>
    api.delete(`/rsvp-events/${id}`),
  activate: (id: string) =>
    api.post(`/rsvp-events/${id}/activate`),
  close: (id: string) =>
    api.post(`/rsvp-events/${id}/close`),
  responses: (id: string, params?: { attendance?: string; page?: number; limit?: number }) =>
    api.get(`/rsvp-events/${id}/responses`, { params }),
  tally: (id: string) =>
    api.get(`/rsvp-events/${id}/tally`),
  sendInvites: (id: string, contactIds: string[]) =>
    api.post(`/rsvp-events/${id}/send-invites`, { contact_ids: contactIds }),
  exportUrl: (id: string) =>
    `${api.defaults.baseURL}/rsvp-events/${id}/export`,
}
```

- [ ] **Step 3: Typecheck**

Run: `cd frontend && npm run build`
Expected: build succeeds (no type errors in the changed files).

- [ ] **Step 4: Commit**

```bash
git add frontend/src/services/api.ts frontend/src/lib/utils.ts
git commit -m "feat(rsvp): frontend api service and dd/mm/yyyy date helper"
```

---

## Task 12: Frontend router + navigation + i18n keys

**Files:**
- Modify: `frontend/src/router/index.ts`
- Modify: `frontend/src/components/layout/navigation.ts`
- Modify: locale files under `frontend/src/i18n/` (mirror keys across all present languages — English shown here)

**Interfaces:**
- Produces routes `rsvp`, `rsvp-new`, `rsvp-edit`, `rsvp-results`; nav entry `nav.rsvp`; i18n `rsvp.*`.

- [ ] **Step 1: Add routes** — inside the authenticated children array in `frontend/src/router/index.ts` (next to the `campaigns` route):

```typescript
      {
        path: 'rsvp',
        name: 'rsvp',
        component: () => import('@/views/rsvp/RSVPEventsView.vue'),
        meta: { permission: 'rsvp' }
      },
      {
        path: 'rsvp/new',
        name: 'rsvp-new',
        component: () => import('@/views/rsvp/RSVPEventBuilderView.vue'),
        meta: { permission: 'rsvp' }
      },
      {
        path: 'rsvp/:id/edit',
        name: 'rsvp-edit',
        component: () => import('@/views/rsvp/RSVPEventBuilderView.vue'),
        meta: { permission: 'rsvp' }
      },
      {
        path: 'rsvp/:id/results',
        name: 'rsvp-results',
        component: () => import('@/views/rsvp/RSVPResultsView.vue'),
        meta: { permission: 'rsvp' }
      },
```

- [ ] **Step 2: Add nav entry** — in `frontend/src/components/layout/navigation.ts`, import an icon and add an item after the campaigns entry:

```typescript
import { CalendarCheck } from 'lucide-vue-next'
```

```typescript
  {
    name: 'nav.rsvp',
    path: '/rsvp',
    icon: CalendarCheck,
    permission: 'rsvp'
  },
```

- [ ] **Step 3: Add i18n keys** — add these keys into the EXISTING objects in each locale file (do not create duplicate `nav`/`resources` objects). English values:

```json
"nav": { "rsvp": "RSVP" },
"rsvp": {
  "title": "RSVP Events",
  "create": "New RSVP",
  "name": "Name",
  "description": "Description",
  "eventDate": "Event date",
  "closeDate": "RSVP close date",
  "keyword": "Keyword",
  "flow": "Question flow",
  "account": "WhatsApp account",
  "inviteTemplate": "Invite template",
  "reminder": "Send reminders",
  "reminderAt": "Reminder time",
  "reminderTemplate": "Reminder template",
  "status": "Status",
  "activate": "Activate",
  "close": "Close",
  "results": "Results",
  "export": "Export",
  "sendInvites": "Send invites",
  "yes": "Yes",
  "no": "No",
  "maybe": "Maybe",
  "pending": "Pending",
  "total": "Total",
  "guest": "Guest",
  "respondedAt": "Responded"
},
"resources": { "RSVP": "RSVP event", "rsvp": "RSVP event" }
```

- [ ] **Step 4: Build**

Run: `cd frontend && npm run build`
Expected: build succeeds (lazy view imports resolve; the views are created in Task 13). If the bundler eagerly resolves the imports, do Task 13 first, then re-run.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/router/index.ts frontend/src/components/layout/navigation.ts frontend/src/i18n
git commit -m "feat(rsvp): routes, navigation entry, and i18n keys"
```

---

## Task 13: Frontend views (list, builder, results)

**Files:**
- Create: `frontend/src/views/rsvp/RSVPEventsView.vue`
- Create: `frontend/src/views/rsvp/RSVPEventBuilderView.vue`
- Create: `frontend/src/views/rsvp/RSVPResultsView.vue`

**Interfaces:**
- Consumes: `rsvpService`, `formatDateDDMMYYYY`, shared components (`PageHeader`, `DataTable`, `SearchInput`, `DeleteConfirmDialog`), `useAuthStore().hasPermission`.

- [ ] **Step 1: Events list view** — `frontend/src/views/rsvp/RSVPEventsView.vue`. Structure mirrors `frontend/src/views/chatbot/ChatbotFlowsView.vue`:

```vue
<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { rsvpService } from '@/services/api'
import { toast } from 'vue-sonner'
import { PageHeader, DataTable, DeleteConfirmDialog, SearchInput, type Column } from '@/components/shared'
import { getErrorMessage } from '@/lib/api-utils'
import { formatDateDDMMYYYY } from '@/lib/utils'
import { Plus, Pencil, Trash2, BarChart3, CalendarCheck } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'

interface RSVPEvent {
  id: string
  name: string
  status: string
  event_date?: string
}

const { t } = useI18n()
const router = useRouter()
const authStore = useAuthStore()
const events = ref<RSVPEvent[]>([])
const isLoading = ref(true)
const searchQuery = ref('')
const deleteOpen = ref(false)
const toDelete = ref<RSVPEvent | null>(null)

const columns = computed<Column<RSVPEvent>[]>(() => [
  { key: 'name', label: t('rsvp.name'), sortable: true },
  { key: 'status', label: t('rsvp.status') },
  { key: 'event_date', label: t('rsvp.eventDate') },
  { key: 'actions', label: '', align: 'right' },
])

async function fetchEvents() {
  isLoading.value = true
  try {
    const res = await rsvpService.list({ search: searchQuery.value || undefined })
    const data = (res.data as any).data || res.data
    events.value = data.events || []
  } catch {
    events.value = []
  } finally {
    isLoading.value = false
  }
}

onMounted(fetchEvents)

function createEvent() { router.push('/rsvp/new') }
function editEvent(e: RSVPEvent) { router.push(`/rsvp/${e.id}/edit`) }
function viewResults(e: RSVPEvent) { router.push(`/rsvp/${e.id}/results`) }
function askDelete(e: RSVPEvent) { toDelete.value = e; deleteOpen.value = true }

async function confirmDelete() {
  if (!toDelete.value) return
  try {
    await rsvpService.delete(toDelete.value.id)
    toast.success(t('common.deletedSuccess', { resource: t('resources.RSVP') }))
    deleteOpen.value = false
    toDelete.value = null
    await fetchEvents()
  } catch (e: any) {
    toast.error(getErrorMessage(e, t('common.failedDelete', { resource: t('resources.rsvp') })))
  }
}
</script>

<template>
  <div class="flex flex-col h-full">
    <PageHeader :title="t('rsvp.title')" :icon="CalendarCheck" back-link="/">
      <template #actions>
        <Button v-if="authStore.hasPermission('rsvp', 'write')" variant="outline" size="sm" @click="createEvent">
          <Plus class="h-4 w-4 mr-2" />{{ t('rsvp.create') }}
        </Button>
      </template>
    </PageHeader>
    <div class="p-6">
      <div class="max-w-6xl mx-auto">
        <SearchInput v-model="searchQuery" :placeholder="t('common.search')" class="mb-4" @update:modelValue="fetchEvents" />
        <DataTable :columns="columns" :rows="events" :loading="isLoading">
          <template #cell-status="{ item }">
            <Badge>{{ t('rsvp.' + item.status) }}</Badge>
          </template>
          <template #cell-event_date="{ item }">
            <span class="text-sm text-muted-foreground">{{ item.event_date ? formatDateDDMMYYYY(item.event_date) : '—' }}</span>
          </template>
          <template #cell-actions="{ item }">
            <Button variant="ghost" size="sm" @click="viewResults(item)"><BarChart3 class="h-4 w-4" /></Button>
            <Button v-if="authStore.hasPermission('rsvp', 'write')" variant="ghost" size="sm" @click="editEvent(item)"><Pencil class="h-4 w-4" /></Button>
            <Button v-if="authStore.hasPermission('rsvp', 'delete')" variant="ghost" size="sm" @click="askDelete(item)"><Trash2 class="h-4 w-4" /></Button>
          </template>
        </DataTable>
      </div>
    </div>
    <DeleteConfirmDialog :open="deleteOpen" :title="t('common.deleteConfirm')" @confirm="confirmDelete" @cancel="deleteOpen = false" />
  </div>
</template>
```

> If shared component import paths/props differ, copy them verbatim from `ChatbotFlowsView.vue`. Do not invent prop names.

- [ ] **Step 2: Builder view** — `frontend/src/views/rsvp/RSVPEventBuilderView.vue`. A form for create/edit with these fields: name, description, keyword (text); event date + RSVP close date (date inputs); linked flow, WhatsApp account, invite template (selects populated from existing services — reuse the same select components + data-loading the campaign create form in `CampaignsView.vue` uses for account/template selection); reminder toggle + reminder time + reminder template. Behavior:
  - On mount, if `route.params.id` exists: `rsvpService.get(id)` and populate the form (display existing dates with `formatDateDDMMYYYY`).
  - Save: build a payload, convert date inputs to ISO strings, then `rsvpService.create(payload)` (new) or `rsvpService.update(id, payload)` (edit); toast success; `router.push('/rsvp')`.
  - When editing, show **Activate** and **Close** buttons that call `rsvpService.activate(id)` / `rsvpService.close(id)`. Handle the 409 keyword conflict with a toast:

```typescript
try {
  await rsvpService.activate(id)
  toast.success(t('rsvp.activate'))
  await load()
} catch (e: any) {
  toast.error(getErrorMessage(e, t('rsvp.activate')))
}
```

  Copy the form/select/date-input component imports verbatim from `CampaignsView.vue`; do not invent component names.

- [ ] **Step 3: Results view** — `frontend/src/views/rsvp/RSVPResultsView.vue`:

```vue
<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { rsvpService } from '@/services/api'
import { formatDateDDMMYYYY } from '@/lib/utils'

const { t } = useI18n()
const route = useRoute()
const id = route.params.id as string
const tally = ref<Record<string, number>>({ yes: 0, no: 0, maybe: 0, pending: 0, total: 0 })
const responses = ref<any[]>([])
let timer: number | undefined

async function loadTally() {
  const res = await rsvpService.tally(id)
  tally.value = (res.data as any).data || res.data
}
async function loadResponses() {
  const res = await rsvpService.responses(id)
  const data = (res.data as any).data || res.data
  responses.value = data.responses || []
}
function exportXlsx() { window.open(rsvpService.exportUrl(id), '_blank') }

onMounted(async () => {
  await Promise.all([loadTally(), loadResponses()])
  timer = window.setInterval(loadTally, 15000)
})
onUnmounted(() => { if (timer) window.clearInterval(timer) })
</script>

<template>
  <div class="p-6 max-w-6xl mx-auto space-y-6">
    <div class="flex items-center justify-between">
      <h1 class="text-xl font-semibold">{{ t('rsvp.results') }}</h1>
      <Button variant="outline" size="sm" @click="exportXlsx">{{ t('rsvp.export') }}</Button>
    </div>
    <div class="grid grid-cols-2 md:grid-cols-5 gap-4">
      <Card v-for="k in ['yes','no','maybe','pending','total']" :key="k">
        <CardHeader><CardTitle class="text-sm text-muted-foreground">{{ t('rsvp.' + k) }}</CardTitle></CardHeader>
        <CardContent><div class="text-2xl font-bold">{{ tally[k] ?? 0 }}</div></CardContent>
      </Card>
    </div>
    <table class="w-full text-sm">
      <thead><tr class="text-left text-muted-foreground">
        <th class="py-2">{{ t('rsvp.guest') }}</th>
        <th>{{ t('rsvp.status') }}</th>
        <th>{{ t('rsvp.respondedAt') }}</th>
      </tr></thead>
      <tbody>
        <tr v-for="row in responses" :key="row.id" class="border-t">
          <td class="py-2">{{ row.contact?.profile_name || row.phone_number }}</td>
          <td>{{ t('rsvp.' + row.attendance) }}</td>
          <td>{{ row.responded_at ? formatDateDDMMYYYY(row.responded_at) : '—' }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
```

- [ ] **Step 4: Build**

Run: `cd frontend && npm run build`
Expected: build succeeds.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/views/rsvp
git commit -m "feat(rsvp): events list, builder, and live results views"
```

---

## Task 14: End-to-end verification

**Files:** none; verification only.

- [ ] **Step 1: Backend green**

Run: `go test ./... 2>&1 | tail -20 && go vet ./... && go build ./...`
Expected: all pass.

- [ ] **Step 2: Frontend green**

Run: `cd frontend && npm run build`
Expected: build succeeds.

- [ ] **Step 3: Manual smoke (documented, run locally)**

1. `make run-migrate` (backend) + `cd frontend && npm run dev`.
2. Log in; confirm **RSVP** appears in the sidebar for an admin/manager.
3. Create an event, attach a chatbot flow whose first question stores `attendance` with button IDs `yes`/`no`/`maybe`, set keyword `TESTRSVP`, activate it.
4. From a WhatsApp test number, message `TESTRSVP`, answer the flow.
5. Open Results — confirm the tally increments and the guest row shows the answer; dates render dd/mm/yyyy.
6. Click Export — confirm an `.xlsx` downloads with the answer columns.

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "chore(rsvp): end-to-end verification pass"
```

---

## Self-Review (author checklist — completed)

- **Spec coverage:** dynamic questions (reuse flow builder + `SessionData`) → Task 6; per-event/per-org flows → Tasks 1/6; concurrent events + unique-active-keyword → Task 4; campaign + keyword entry → Tasks 6/7; live dashboard + guest list → Tasks 5/13; XLSX export → Task 9; reminders → Task 8; cutoff/close → Tasks 4/6; permissions → Task 2; edge cases (double reply upsert → Task 6, unknown sender via existing `GetOrCreateContact` in the processor's ingest path → Task 6 hook, reply-after-cutoff → Task 6, flow edited mid-event via JSONB → Task 6).
- **Naming consistency:** `RSVPEvent`/`RSVPResponse`, `finalizeRSVPFromSession`, `seedPendingRSVPResponse`, `rsvpEventForFlow`, `syncRSVPFlowKeyword`, `sendApprovedTemplate` (same placeholder in Tasks 7 & 8 — resolve to the real campaign send helper once, use identically), `rsvpService`, routes `/api/rsvp-events/...` — consistent across tasks.
- **Placeholders requiring lookup during implementation (called out inline, not plan gaps):** the exact single-recipient template-send helper (`sendApprovedTemplate`, Tasks 7/8) and the precise flow-completion call site (Task 6) must be confirmed against the real code; each has an inline note on how to find it.
- **Stub ordering:** `syncRSVPFlowKeyword` is introduced as a compile-safe stub in Task 4 and given its real body in Task 6, so every task compiles on its own.
