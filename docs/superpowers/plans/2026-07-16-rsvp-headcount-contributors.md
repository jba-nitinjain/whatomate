# RSVP Headcount Contributors — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the hardcoded spouse tally with a per-event, configurable list of "headcount contributors", so a children count (or anything else) can be counted and totalled without children-specific code.

**Architecture:** A new `RSVPHeadcountContributors` value stored as JSONB on `rsvp_events` describes which answer keys contribute people to the headcount and how (`boolean` = 1 per match, `numeric` = the value given). `GetRSVPTally` computes cards from that list instead of the literal `spouse_attendance`, and adds a grand total. The `spouse_attendance` string is currently hardcoded in **three** places — Go tally, raw SQL guest filter, Vue label — all three move to config.

**Tech Stack:** Go 1.x, GORM (AutoMigrate — there are no SQL migration files), PostgreSQL jsonb, fastglue, Vue 3 + TypeScript.

## Global Constraints

- **Spec:** `docs/superpowers/specs/2026-07-16-rsvp-headcount-and-followup-design.md` — Sections 2 and 3.
- **Do NOT depend on** the reminder-send-fix plan (`2026-07-16-rsvp-reminder-send-fix.md`) or the follow-up plan. That plan must stay independently deployable; this one must not entangle it.
- **Backwards compatibility is mandatory.** The live event has 1,276 guests mid-flight, event date **19/07/2026**. An event with no contributors configured must behave exactly as today. Seed defaults; never require reconfiguration of a running event.
- **Schema changes go through GORM AutoMigrate** (`internal/database/postgres.go:82-85`). There are no migration files — do not create a migrations directory.
- **Go tests:** stdlib `testing` + testify v1.11.1. Pure-logic tests use `package handlers` and plain `t.Fatalf` (see `rsvp_tally_test.go`); DB tests use `package handlers_test` + `testutil.SetupTestDB(t)`, which **skips** unless `TEST_DATABASE_URL` is set.
- **Dates displayed to users must be dd/mm/yyyy.** Store/transmit ISO.

---

## Verified Facts (do not re-derive)

Read from the codebase on 2026-07-16.

| Fact | Location |
| --- | --- |
| `func buildRSVPAttendanceBreakdown(responses []models.RSVPResponse, spouseMobileField string) rsvpAttendanceBreakdown` | `internal/handlers/rsvp_tally.go:45` |
| **Hardcode #1** — `normalizedRSVPAnswer(response.Answers, "spouse_attendance", "spouse_attendance_title")` | `rsvp_tally.go:52` |
| `func normalizedRSVPAnswer(answers models.JSONB, keys ...string) string` — lowercases, trims, first non-empty wins | `rsvp_tally.go:22` |
| `func addAttendanceCount(counts *rsvpAttendanceCounts, value string)` — `yes\|attending`, `no\|not attending\|not_attending`, `maybe`, default `pending` | `rsvp_tally.go:32` |
| `rsvpAttendanceCounts{Attending, NotAttending, Maybe, Pending int}` | `rsvp_tally.go:9-14` |
| `rsvpAttendanceBreakdown{Member, Spouse rsvpAttendanceCounts}` | `rsvp_tally.go:16-19` |
| `func (a *App) GetRSVPTally(r *fastglue.Request) error` | `internal/handlers/rsvp.go:480-554` |
| Tally envelope keys: `yes,no,maybe,pending,total,breakdowns,attendance_field,member_attendance,spouse_attendance` | `rsvp.go:546-553` |
| Dynamic `*_title` breakdown via `jsonb_each_text` + `right(je.key,6)='_title'` | `rsvp.go:510-533` |
| **Hardcode #2** — spouse SQL filter, same literal key pair | `internal/handlers/rsvp_guests.go:168-186` |
| `rsvpGuestRosterRow` | `rsvp_guests.go:126-131` |
| `RSVPEvent` struct (has `AttendanceField`, `AttendanceMap`, `SpouseMobileField`; **no** spouse-attendance key config) | `internal/models/rsvp.go:66-113` |
| `RSVPResponse` struct — `Attendance RSVPAttendance`, `Answers JSONB` | `internal/models/rsvp.go:116-136` |
| `RSVPAttendance` constants: `pending,yes,no,maybe` | `internal/models/rsvp.go:17-24` |
| `models.JSONB` is a **map** type (`answers[key]`) — cannot hold a JSON array | `internal/models/` |
| AutoMigrate registration | `internal/database/postgres.go:82-85` |
| **Hardcode #3** — `if (k === 'spouse_attendance_title') return t('rsvp.spouseAttendance')` | `frontend/src/views/rsvp/RSVPResultsView.vue:121` |
| `answerKeys` computed — filters only `_`-prefixed keys | `RSVPResultsView.vue:109-117` |
| `columns` computed — one column per answer key | `RSVPResultsView.vue:125-139` |
| `toggleCardFilter` drives `member_status`/`spouse_status` query params | `RSVPResultsView.vue:87-93` |

### Two behaviours to preserve exactly

**1. The spouse "pending" double-count is intentional.** `rsvp_tally.go:53-61`: a spouse marked attending whose mobile has fewer than 10 digits is counted in **both** `Attending` and `Pending`, per the comment *"Attendance and completion are independent"*. This is why the live cards read Yes 207 / No 29 / Pending 1077 = 1313 against a total of 1276. **Cards are not expected to sum to the total.** Do not "fix" this.

**2. Member and spouse counts come from different sources.** Member counts read the `attendance` **column** (`string(response.Attendance)`); spouse counts read the **answers JSONB**. Both funnel into the same `addAttendanceCount` switch. `AttendanceMap` normalization is applied only to the member field at capture time (`rsvp_capture.go`), which is exactly why the tally has to guess at `"yes"|"attending"` for the spouse.

---

## File Structure

| File | Responsibility | Change |
| --- | --- | --- |
| `internal/models/rsvp.go` | RSVP schema | Add `RSVPHeadcountContributor`, `RSVPHeadcountContributors` (Scanner/Valuer), field on `RSVPEvent` |
| `internal/models/rsvp_headcount_test.go` | Scanner/Valuer round-trip | Create |
| `internal/handlers/rsvp_headcount.go` | Numeric parsing + contributor evaluation | Create |
| `internal/handlers/rsvp_headcount_test.go` | Parsing + evaluation tests | Create |
| `internal/handlers/rsvp_tally.go` | Tally computation | Modify: drive from contributors; keep legacy shape |
| `internal/handlers/rsvp_tally_test.go` | Existing tally tests | Modify: extend, keep green |
| `internal/handlers/rsvp.go` | Tally endpoint | Modify: return `contributors` + `total_attending` |
| `internal/handlers/rsvp_guests.go` | Guest filters | Modify: generic contributor filter |
| `frontend/src/views/rsvp/RSVPResultsView.vue` | Results dashboard | Modify: dynamic cards, total, de-duped columns |
| `frontend/src/views/rsvp/RSVPEventBuilderView.vue` | Event settings | Modify: contributor editor |

---

## Task 1: Fix the duplicated results columns (Section 2)

The chatbot writes both `spouse_attendance` (`"yes"`) and `spouse_attendance_title` (`"Attending"`) into `answers`. `answerKeys` (`RSVPResultsView.vue:113`) filters only `_`-prefixed keys, so both survive; `columns` (`:133`) gives each its own column. `prettyKey("spouse_attendance_title")` hits the `:121` branch → "Spouse Attendance"; `prettyKey("spouse_attendance")` falls through to `:122` → "Spouse Attendance". **Two columns, identical label, different values.**

The same collision exists for the member pair (`attendance` → "Attendance", `attendance_title` → "Member Attendance") — different labels, but the raw column is equally redundant. Live grid today: `Status | Attendance | Member Attendance | Spouse Attendance | Spouse Attendance | Spouse Mobile`.

The column **keys** differ (`answers.spouse_attendance` vs `answers.spouse_attendance_title`), so this cannot be fixed by renaming a label — a key must be dropped from `answerKeys`.

This task is standalone and shippable on its own.

**Files:**
- Modify: `frontend/src/views/rsvp/RSVPResultsView.vue:109-117`
- Test: `frontend/e2e/tests/rsvp-results-columns.spec.ts` (create)

**Interfaces:**
- Consumes: nothing.
- Produces: nothing (Vue-internal).

- [ ] **Step 1: Write the failing e2e test**

Create `frontend/e2e/tests/rsvp-results-columns.spec.ts`. Follow the existing conventions in `frontend/e2e/tests/` — read a neighbouring spec and copy its auth/setup helper usage rather than inventing one.

```ts
import { test, expect } from '@playwright/test'

test('results grid shows each answer question exactly once', async ({ page }) => {
  await page.goto(`/rsvp/${process.env.RSVP_EVENT_ID}/results`)

  const headers = await page.locator('table thead th').allInnerTexts()
  const meaningful = headers.map(h => h.trim()).filter(Boolean)
  const duplicates = meaningful.filter((h, i) => meaningful.indexOf(h) !== i)

  expect(duplicates, `duplicate column headers: ${duplicates.join(', ')}`).toEqual([])
})
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd frontend && BASE_URL=<dev-url> RSVP_EVENT_ID=<event-id> npx playwright test rsvp-results-columns --project=chromium
```

Expected: FAIL — `duplicate column headers: Spouse Attendance`.

- [ ] **Step 3: Drop the raw key when a `_title` companion exists**

In `RSVPResultsView.vue`, replace the `answerKeys` computed at `:109-117`:

```ts
// Union of answer keys across all responses (first-seen order), one column each.
// The chatbot writes both `<key>` (raw value, e.g. "yes") and `<key>_title`
// (display value, e.g. "Attending"). Showing both produced two columns with the
// same label and different values, so the raw key is dropped wherever its
// _title companion is present.
const answerKeys = computed<string[]>(() => {
  const seen: string[] = []
  for (const row of responses.value) {
    for (const k of Object.keys(row.answers || {})) {
      if (!k.startsWith('_') && !seen.includes(k)) seen.push(k)
    }
  }
  const titled = new Set(
    seen.filter(k => k.endsWith('_title')).map(k => k.slice(0, -'_title'.length)),
  )
  return seen.filter(k => !titled.has(k))
})
```

- [ ] **Step 4: Run to verify it passes**

```bash
cd frontend && BASE_URL=<dev-url> RSVP_EVENT_ID=<event-id> npx playwright test rsvp-results-columns --project=chromium
```

Expected: PASS. Grid becomes `Status | Member Attendance | Spouse Attendance | Spouse Mobile`.

- [ ] **Step 5: Confirm export is unaffected**

Click **Export** on the results page. The export is built server-side (`ExportRSVPResponses`, `rsvp.go:679`) and must be unchanged by this frontend edit — confirm both raw and title values still appear in the file. If the user wants the export de-duped too, that is a separate change; do not do it here.

- [ ] **Step 6: Typecheck, lint, commit**

```bash
cd frontend && npm run typecheck && npm run lint
cd .. && git add frontend/src/views/rsvp/RSVPResultsView.vue frontend/e2e/tests/rsvp-results-columns.spec.ts
git commit -m "Show each RSVP answer question once in the results grid

The chatbot writes both <key> and <key>_title into answers, and both
became columns - so 'Spouse Attendance' appeared twice with the same
label and different values. Drop the raw key where its _title companion
exists."
```

---

## Task 2: The contributor model

**Files:**
- Modify: `internal/models/rsvp.go`
- Test: `internal/models/rsvp_headcount_test.go` (create)

**Interfaces:**
- Consumes: nothing.
- Produces — used by every later task:
  ```go
  type RSVPHeadcountMode string
  const (
      RSVPHeadcountModeBoolean RSVPHeadcountMode = "boolean"
      RSVPHeadcountModeNumeric RSVPHeadcountMode = "numeric"
  )

  type RSVPHeadcountContributor struct {
      Label       string            `json:"label"`
      AnswerKey   string            `json:"answer_key"`
      Mode        RSVPHeadcountMode `json:"mode"`
      MatchValues []string          `json:"match_values,omitempty"`
  }

  type RSVPHeadcountContributors []RSVPHeadcountContributor
  func (c RSVPHeadcountContributors) Value() (driver.Value, error)
  func (c *RSVPHeadcountContributors) Scan(value interface{}) error
  ```
  Plus field on `RSVPEvent`:
  ```go
  HeadcountContributors RSVPHeadcountContributors `gorm:"type:jsonb;default:'[]'" json:"headcount_contributors"`
  ```

`models.JSONB` is a map type and cannot hold an array, hence the dedicated slice type with Scanner/Valuer.

- [ ] **Step 1: Write the failing test**

Create `internal/models/rsvp_headcount_test.go`:

```go
package models

import (
	"testing"
)

func TestRSVPHeadcountContributorsRoundTrip(t *testing.T) {
	original := RSVPHeadcountContributors{
		{Label: "Member attendance", AnswerKey: "attendance", Mode: RSVPHeadcountModeBoolean, MatchValues: []string{"yes", "attending"}},
		{Label: "Children", AnswerKey: "children_count", Mode: RSVPHeadcountModeNumeric},
	}

	value, err := original.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}

	var restored RSVPHeadcountContributors
	if err := restored.Scan(value); err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if len(restored) != 2 {
		t.Fatalf("expected 2 contributors, got %d", len(restored))
	}
	if restored[0].Label != "Member attendance" || restored[0].Mode != RSVPHeadcountModeBoolean {
		t.Fatalf("first contributor corrupted: %+v", restored[0])
	}
	if len(restored[0].MatchValues) != 2 || restored[0].MatchValues[0] != "yes" {
		t.Fatalf("match values corrupted: %+v", restored[0].MatchValues)
	}
	if restored[1].AnswerKey != "children_count" || restored[1].Mode != RSVPHeadcountModeNumeric {
		t.Fatalf("second contributor corrupted: %+v", restored[1])
	}
}

func TestRSVPHeadcountContributorsScanNil(t *testing.T) {
	// A row written before this column existed scans as NULL and must yield an
	// empty list, not an error - the live event predates this feature.
	var c RSVPHeadcountContributors
	if err := c.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) must not error: %v", err)
	}
	if len(c) != 0 {
		t.Fatalf("Scan(nil) must yield empty, got %+v", c)
	}
}

func TestRSVPHeadcountContributorsScanEmptyArray(t *testing.T) {
	var c RSVPHeadcountContributors
	if err := c.Scan([]byte(`[]`)); err != nil {
		t.Fatalf("Scan([]) error: %v", err)
	}
	if len(c) != 0 {
		t.Fatalf("expected empty, got %+v", c)
	}
}

func TestRSVPHeadcountContributorsScanGarbage(t *testing.T) {
	var c RSVPHeadcountContributors
	if err := c.Scan([]byte(`{"not":"an array"}`)); err == nil {
		t.Fatal("expected an error scanning a non-array")
	}
}

func TestRSVPHeadcountContributorsValueEmptyIsArrayNotNull(t *testing.T) {
	// Must serialize to [] so the jsonb column default and the API shape agree;
	// a null would make the frontend guard for two empty representations.
	var c RSVPHeadcountContributors
	v, err := c.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	if string(v.([]byte)) != "[]" {
		t.Fatalf("empty contributors must serialize to [], got %s", v.([]byte))
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/models/ -run TestRSVPHeadcountContributors -v
```

Expected: FAIL to build — undefined types.

- [ ] **Step 3: Implement**

Add to `internal/models/rsvp.go`. Ensure `database/sql/driver`, `encoding/json` and `fmt` are imported.

```go
// RSVPHeadcountMode selects how a contributor's answer becomes a number of people.
type RSVPHeadcountMode string

const (
	// RSVPHeadcountModeBoolean counts 1 when the answer matches MatchValues.
	RSVPHeadcountModeBoolean RSVPHeadcountMode = "boolean"
	// RSVPHeadcountModeNumeric counts the number the guest gave.
	RSVPHeadcountModeNumeric RSVPHeadcountMode = "numeric"
)

// RSVPHeadcountContributor declares that one answer key contributes people to the
// event headcount. This replaces the hardcoded spouse_attendance tally: children,
// drivers, or anything else is configuration, not code.
type RSVPHeadcountContributor struct {
	Label       string            `json:"label"`
	AnswerKey   string            `json:"answer_key"`
	Mode        RSVPHeadcountMode `json:"mode"`
	MatchValues []string          `json:"match_values,omitempty"`
}

// RSVPHeadcountContributors is an ordered list stored as jsonb. models.JSONB is a
// map type and cannot hold an array, so this carries its own Scanner/Valuer.
type RSVPHeadcountContributors []RSVPHeadcountContributor

func (c RSVPHeadcountContributors) Value() (driver.Value, error) {
	if c == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(c)
}

func (c *RSVPHeadcountContributors) Scan(value interface{}) error {
	if value == nil {
		*c = RSVPHeadcountContributors{}
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into RSVPHeadcountContributors", value)
	}
	if len(data) == 0 {
		*c = RSVPHeadcountContributors{}
		return nil
	}
	return json.Unmarshal(data, c)
}
```

Then add the field to `RSVPEvent`, beside `SpouseMobileField`:

```go
	// Headcount contributors: which answers add people to the event headcount and
	// how. Empty means legacy behaviour (member + spouse), so events created before
	// this existed keep working untouched.
	HeadcountContributors RSVPHeadcountContributors `gorm:"type:jsonb;default:'[]'" json:"headcount_contributors"`
```

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/models/ -run TestRSVPHeadcountContributors -v
```

Expected: all six PASS.

- [ ] **Step 5: Confirm AutoMigrate adds the column without disturbing live data**

`RSVPEvent` is already registered at `internal/database/postgres.go:82-85`, so no registration change is needed. With `TEST_DATABASE_URL` set:

```bash
go test ./internal/handlers/ -run TestRSVPModels_Migrate_And_CRUD -v
```

Expected: PASS — proves AutoMigrate adds `headcount_contributors` cleanly.

- [ ] **Step 6: Commit**

```bash
git add internal/models/rsvp.go internal/models/rsvp_headcount_test.go
git commit -m "Add RSVP headcount contributor model

An ordered, per-event list describing which answer keys contribute people
to the headcount and how (boolean = 1 per match, numeric = the value
given). Carries its own Scanner/Valuer because models.JSONB is a map type
and cannot hold an array.

Empty means legacy behaviour, so the in-flight event is untouched."
```

---

## Task 3: Numeric parsing

Free text is permitted for the count question, so parsing must be lenient but never silently lose a family. Unparseable input counts as 0 **and is flagged**.

**Files:**
- Create: `internal/handlers/rsvp_headcount.go`
- Test: `internal/handlers/rsvp_headcount_test.go`

**Interfaces:**
- Consumes: `models.RSVPHeadcountContributor` (Task 2).
- Produces:
  - `func parseHeadcountValue(raw string) (value int, ok bool)` — `ok=false` means unparseable; `value` is then 0.
  - `const headcountReviewCeiling = 20`
  - `func headcountNeedsReview(value int) bool`

- [ ] **Step 1: Write the failing test**

Create `internal/handlers/rsvp_headcount_test.go`:

```go
package handlers

import "testing"

func TestParseHeadcountValue(t *testing.T) {
	cases := []struct {
		in    string
		value int
		ok    bool
	}{
		{"3", 3, true},
		{"0", 0, true},
		{"3 kids", 3, true},
		{"we are bringing 2 children", 2, true},
		{"three", 3, true},
		{"Three", 3, true},
		{"TWO", 2, true},
		{"zero", 0, true},
		{"ten", 10, true},
		{"", 0, true},
		{"   ", 0, true},
		{"no", 0, true},
		{"none", 0, true},
		{"nil", 0, true},
		{"-1", 0, true},   // clamped, not rejected
		{"999", 999, true}, // parsed; flagged separately by headcountNeedsReview
		{"abc", 0, false},
		{"many", 0, false},
		{"a few", 0, false},
	}
	for _, c := range cases {
		value, ok := parseHeadcountValue(c.in)
		if value != c.value || ok != c.ok {
			t.Errorf("parseHeadcountValue(%q) = (%d, %v), want (%d, %v)", c.in, value, ok, c.value, c.ok)
		}
	}
}

func TestHeadcountNeedsReview(t *testing.T) {
	if headcountNeedsReview(3) {
		t.Error("3 must not need review")
	}
	if headcountNeedsReview(20) {
		t.Error("20 is the ceiling and must not need review")
	}
	if !headcountNeedsReview(21) {
		t.Error("21 must need review")
	}
	if !headcountNeedsReview(999) {
		t.Error("999 must need review")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/handlers/ -run 'TestParseHeadcountValue|TestHeadcountNeedsReview' -v
```

Expected: FAIL to build — undefined.

- [ ] **Step 3: Implement**

Create `internal/handlers/rsvp_headcount.go`:

```go
package handlers

import (
	"regexp"
	"strconv"
	"strings"
)

// headcountReviewCeiling is the largest count accepted without being flagged for a
// human look. Above it the value is still counted - it is flagged, not rejected.
const headcountReviewCeiling = 20

var headcountDigitsPattern = regexp.MustCompile(`\d+`)

// headcountWords maps the number words a guest might type. Kept deliberately small:
// beyond ten, a typed word is more likely a typo than a real count.
var headcountWords = map[string]int{
	"zero": 0, "one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
	"six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
}

// headcountNoneWords are answers that explicitly mean zero.
var headcountNoneWords = map[string]struct{}{
	"no": {}, "none": {}, "nil": {}, "nope": {}, "n/a": {}, "na": {},
}

// parseHeadcountValue reads a guest's free-text count leniently. ok=false means the
// answer could not be understood; the caller counts 0 and flags the row rather than
// silently losing a family.
func parseHeadcountValue(raw string) (int, bool) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return 0, true
	}
	if _, none := headcountNoneWords[s]; none {
		return 0, true
	}
	if match := headcountDigitsPattern.FindString(s); match != "" {
		n, err := strconv.Atoi(match)
		if err != nil {
			return 0, false
		}
		if n < 0 {
			return 0, true
		}
		return n, true
	}
	for word, n := range headcountWords {
		if strings.Contains(s, word) {
			return n, true
		}
	}
	return 0, false
}

// headcountNeedsReview reports whether a parsed count is implausible enough to show
// to a human. The value still counts.
func headcountNeedsReview(value int) bool {
	return value > headcountReviewCeiling
}
```

Note `"-1"` yields `0, true`: the digits pattern matches `1`, and the sign is not captured — so the clamp is incidental. The test pins the behaviour; leave the simpler code.

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/handlers/ -run 'TestParseHeadcountValue|TestHeadcountNeedsReview' -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/handlers/rsvp_headcount.go internal/handlers/rsvp_headcount_test.go
git commit -m "Add lenient headcount value parsing

The count question may be free text, so accept '3', '3 kids', 'three',
'none' and empty. Unparseable answers count as 0 and report ok=false so
the caller can flag the row rather than silently losing a family. Counts
above 20 are flagged for review, not rejected."
```

---

## Task 4: Evaluate contributors against a response

**Files:**
- Modify: `internal/handlers/rsvp_headcount.go`
- Modify: `internal/handlers/rsvp_headcount_test.go`

**Interfaces:**
- Consumes: `parseHeadcountValue` (Task 3), `normalizedRSVPAnswer` (`rsvp_tally.go:22`), `models.RSVPHeadcountContributor` (Task 2).
- Produces:
  ```go
  type headcountContribution struct {
      People      int
      Matched     bool
      NeedsReview bool
      Unparseable bool
  }
  func evaluateHeadcountContributor(c models.RSVPHeadcountContributor, answers models.JSONB, attendance models.RSVPAttendance) headcountContribution
  func legacyHeadcountContributors(attendanceField string) models.RSVPHeadcountContributors
  ```

- [ ] **Step 1: Write the failing test**

Append to `internal/handlers/rsvp_headcount_test.go`:

```go
func TestEvaluateHeadcountContributorBoolean(t *testing.T) {
	c := models.RSVPHeadcountContributor{
		Label: "Spouse", AnswerKey: "spouse_attendance",
		Mode: models.RSVPHeadcountModeBoolean, MatchValues: []string{"yes", "attending"},
	}

	got := evaluateHeadcountContributor(c, models.JSONB{"spouse_attendance": "yes"}, models.RSVPAttendanceYes)
	if got.People != 1 || !got.Matched {
		t.Fatalf("expected 1 person matched, got %+v", got)
	}

	// The _title companion must satisfy the same contributor.
	got = evaluateHeadcountContributor(c, models.JSONB{"spouse_attendance_title": "Attending"}, models.RSVPAttendanceYes)
	if got.People != 1 || !got.Matched {
		t.Fatalf("_title companion must match, got %+v", got)
	}

	got = evaluateHeadcountContributor(c, models.JSONB{"spouse_attendance": "no"}, models.RSVPAttendanceYes)
	if got.People != 0 || got.Matched {
		t.Fatalf("expected no match, got %+v", got)
	}

	got = evaluateHeadcountContributor(c, models.JSONB{}, models.RSVPAttendanceYes)
	if got.People != 0 || got.Matched {
		t.Fatalf("absent answer must not match, got %+v", got)
	}
}

func TestEvaluateHeadcountContributorBooleanIsCaseInsensitive(t *testing.T) {
	c := models.RSVPHeadcountContributor{
		AnswerKey: "spouse_attendance", Mode: models.RSVPHeadcountModeBoolean,
		MatchValues: []string{"Yes", "ATTENDING"},
	}
	got := evaluateHeadcountContributor(c, models.JSONB{"spouse_attendance": "  yes  "}, models.RSVPAttendanceYes)
	if got.People != 1 {
		t.Fatalf("matching must be case- and space-insensitive, got %+v", got)
	}
}

func TestEvaluateHeadcountContributorNumeric(t *testing.T) {
	c := models.RSVPHeadcountContributor{
		Label: "Children", AnswerKey: "children_count", Mode: models.RSVPHeadcountModeNumeric,
	}

	got := evaluateHeadcountContributor(c, models.JSONB{"children_count": "3"}, models.RSVPAttendanceYes)
	if got.People != 3 || !got.Matched {
		t.Fatalf("expected 3, got %+v", got)
	}

	got = evaluateHeadcountContributor(c, models.JSONB{"children_count": "abc"}, models.RSVPAttendanceYes)
	if got.People != 0 || !got.Unparseable {
		t.Fatalf("unparseable must count 0 and flag, got %+v", got)
	}

	got = evaluateHeadcountContributor(c, models.JSONB{"children_count": "50"}, models.RSVPAttendanceYes)
	if got.People != 50 || !got.NeedsReview {
		t.Fatalf("50 must count but flag for review, got %+v", got)
	}

	got = evaluateHeadcountContributor(c, models.JSONB{}, models.RSVPAttendanceYes)
	if got.People != 0 || got.Unparseable {
		t.Fatalf("absent answer is 0 and not a parse failure, got %+v", got)
	}
}

func TestEvaluateHeadcountContributorIgnoresNonAttendingMember() {
}

func TestLegacyHeadcountContributors(t *testing.T) {
	// Events predating this feature must tally exactly as before.
	got := legacyHeadcountContributors("attendance")
	if len(got) != 2 {
		t.Fatalf("expected member + spouse, got %d: %+v", len(got), got)
	}
	if got[0].AnswerKey != "attendance" || got[0].Mode != models.RSVPHeadcountModeBoolean {
		t.Fatalf("first must be member attendance: %+v", got[0])
	}
	if got[1].AnswerKey != "spouse_attendance" || got[1].Mode != models.RSVPHeadcountModeBoolean {
		t.Fatalf("second must be spouse attendance: %+v", got[1])
	}
}
```

Delete the empty `TestEvaluateHeadcountContributorIgnoresNonAttendingMember` stub above — it is a placeholder and placeholders are plan failures. Replace it with nothing; member gating is the tally's job (Task 5), not the evaluator's.

Add `"github.com/nikyjain/whatomate/internal/models"` to the test file's imports.

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/handlers/ -run 'TestEvaluateHeadcountContributor|TestLegacyHeadcountContributors' -v
```

Expected: FAIL to build — undefined.

- [ ] **Step 3: Implement**

Append to `internal/handlers/rsvp_headcount.go`:

```go
// headcountContribution is one contributor's verdict for one response.
type headcountContribution struct {
	People      int
	Matched     bool
	NeedsReview bool
	Unparseable bool
}

// evaluateHeadcountContributor reads a contributor's answer from a response.
// It checks both `<key>` and `<key>_title`, because the chatbot writes the raw
// value to one and the display value to the other, and a flow author may map
// either.
func evaluateHeadcountContributor(c models.RSVPHeadcountContributor, answers models.JSONB, attendance models.RSVPAttendance) headcountContribution {
	raw := normalizedRSVPAnswer(answers, c.AnswerKey, c.AnswerKey+"_title")

	switch c.Mode {
	case models.RSVPHeadcountModeNumeric:
		if raw == "" {
			return headcountContribution{}
		}
		value, ok := parseHeadcountValue(raw)
		if !ok {
			return headcountContribution{Unparseable: true}
		}
		return headcountContribution{
			People:      value,
			Matched:     value > 0,
			NeedsReview: headcountNeedsReview(value),
		}

	default: // boolean
		if raw == "" {
			return headcountContribution{}
		}
		for _, want := range c.MatchValues {
			if raw == strings.ToLower(strings.TrimSpace(want)) {
				return headcountContribution{People: 1, Matched: true}
			}
		}
		return headcountContribution{}
	}
}

// legacyHeadcountContributors reproduces the pre-configuration behaviour for events
// that have none set: member attendance plus the spouse_attendance key that used to
// be hardcoded at rsvp_tally.go:52.
func legacyHeadcountContributors(attendanceField string) models.RSVPHeadcountContributors {
	if strings.TrimSpace(attendanceField) == "" {
		attendanceField = "attendance"
	}
	return models.RSVPHeadcountContributors{
		{
			Label:       "Member attendance",
			AnswerKey:   attendanceField,
			Mode:        models.RSVPHeadcountModeBoolean,
			MatchValues: []string{"yes", "attending"},
		},
		{
			Label:       "Spouse attendance",
			AnswerKey:   "spouse_attendance",
			Mode:        models.RSVPHeadcountModeBoolean,
			MatchValues: []string{"yes", "attending"},
		},
	}
}
```

Add `"github.com/nikyjain/whatomate/internal/models"` to the file's imports.

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/handlers/ -run 'TestEvaluateHeadcountContributor|TestLegacyHeadcountContributors' -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/handlers/rsvp_headcount.go internal/handlers/rsvp_headcount_test.go
git commit -m "Evaluate headcount contributors against a response

Boolean contributors count 1 on a match; numeric contributors count the
value given. Both <key> and <key>_title are checked, because the chatbot
writes raw and display values to separate keys.

legacyHeadcountContributors reproduces the previously hardcoded member +
spouse tally for events with no configuration."
```

---

## Task 5: Drive the tally from contributors

`buildRSVPAttendanceBreakdown` (`rsvp_tally.go:45`) must keep returning today's `member_attendance` / `spouse_attendance` shape — the frontend depends on it and the event is live — while gaining a contributor-driven total.

**Preserve the intentional double-count** (`rsvp_tally.go:53-61`): a spouse attending with a <10-digit mobile counts in both Attending and Pending. The existing `rsvp_tally_test.go` pins it (`Spouse.Attending != 2 || Spouse.Pending != 3`). Keep those tests green.

**Files:**
- Modify: `internal/handlers/rsvp_tally.go`
- Modify: `internal/handlers/rsvp_tally_test.go`

**Interfaces:**
- Consumes: `evaluateHeadcountContributor`, `legacyHeadcountContributors` (Task 4).
- Produces:
  ```go
  type rsvpContributorTally struct {
      Label       string `json:"label"`
      AnswerKey   string `json:"answer_key"`
      Mode        string `json:"mode"`
      People      int    `json:"people"`
      Responses   int    `json:"responses"`
      NeedsReview int    `json:"needs_review"`
      Unparseable int    `json:"unparseable"`
  }
  func buildRSVPHeadcount(responses []models.RSVPResponse, contributors models.RSVPHeadcountContributors) (tallies []rsvpContributorTally, totalAttending int)
  ```
  Task 6 renders these.

- [ ] **Step 1: Write the failing test**

Append to `internal/handlers/rsvp_tally_test.go`:

```go
func TestBuildRSVPHeadcount(t *testing.T) {
	contributors := models.RSVPHeadcountContributors{
		{Label: "Member", AnswerKey: "attendance", Mode: models.RSVPHeadcountModeBoolean, MatchValues: []string{"yes"}},
		{Label: "Spouse", AnswerKey: "spouse_attendance", Mode: models.RSVPHeadcountModeBoolean, MatchValues: []string{"yes"}},
		{Label: "Children", AnswerKey: "children_count", Mode: models.RSVPHeadcountModeNumeric},
	}
	responses := []models.RSVPResponse{
		{Attendance: models.RSVPAttendanceYes, Answers: models.JSONB{"attendance": "yes", "spouse_attendance": "yes", "children_count": "2"}},
		{Attendance: models.RSVPAttendanceYes, Answers: models.JSONB{"attendance": "yes", "spouse_attendance": "no", "children_count": "1"}},
		{Attendance: models.RSVPAttendanceNo, Answers: models.JSONB{"attendance": "no"}},
	}

	tallies, total := buildRSVPHeadcount(responses, contributors)

	if len(tallies) != 3 {
		t.Fatalf("expected 3 tallies, got %d", len(tallies))
	}
	if tallies[0].People != 2 {
		t.Errorf("member people = %d, want 2", tallies[0].People)
	}
	if tallies[1].People != 1 {
		t.Errorf("spouse people = %d, want 1", tallies[1].People)
	}
	if tallies[2].People != 3 {
		t.Errorf("children people = %d, want 3", tallies[2].People)
	}
	// 2 members + 1 spouse + 3 children
	if total != 6 {
		t.Errorf("total = %d, want 6", total)
	}
}

func TestBuildRSVPHeadcountFlagsBadNumbers(t *testing.T) {
	contributors := models.RSVPHeadcountContributors{
		{Label: "Children", AnswerKey: "children_count", Mode: models.RSVPHeadcountModeNumeric},
	}
	responses := []models.RSVPResponse{
		{Attendance: models.RSVPAttendanceYes, Answers: models.JSONB{"children_count": "2"}},
		{Attendance: models.RSVPAttendanceYes, Answers: models.JSONB{"children_count": "lots"}},
		{Attendance: models.RSVPAttendanceYes, Answers: models.JSONB{"children_count": "50"}},
	}

	tallies, total := buildRSVPHeadcount(responses, contributors)

	if tallies[0].Unparseable != 1 {
		t.Errorf("unparseable = %d, want 1", tallies[0].Unparseable)
	}
	if tallies[0].NeedsReview != 1 {
		t.Errorf("needs review = %d, want 1", tallies[0].NeedsReview)
	}
	// "lots" contributes 0, and is not silently dropped from the flag counts.
	if total != 52 {
		t.Errorf("total = %d, want 52", total)
	}
}

func TestBuildRSVPHeadcountNoContributorsIsZero(t *testing.T) {
	tallies, total := buildRSVPHeadcount([]models.RSVPResponse{
		{Attendance: models.RSVPAttendanceYes, Answers: models.JSONB{"attendance": "yes"}},
	}, models.RSVPHeadcountContributors{})

	if len(tallies) != 0 || total != 0 {
		t.Fatalf("no contributors must yield no tallies and zero total, got %+v / %d", tallies, total)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/handlers/ -run TestBuildRSVPHeadcount -v
```

Expected: FAIL to build — `undefined: buildRSVPHeadcount`.

- [ ] **Step 3: Implement**

Append to `internal/handlers/rsvp_tally.go`:

```go
// rsvpContributorTally is one configured contributor's totals across all responses.
type rsvpContributorTally struct {
	Label       string `json:"label"`
	AnswerKey   string `json:"answer_key"`
	Mode        string `json:"mode"`
	People      int    `json:"people"`
	Responses   int    `json:"responses"`
	NeedsReview int    `json:"needs_review"`
	Unparseable int    `json:"unparseable"`
}

// buildRSVPHeadcount tallies every configured contributor and sums the grand total.
// This replaces the hardcoded spouse_attendance logic: children, drivers or anything
// else are rows in the event's configuration, not branches in this function.
func buildRSVPHeadcount(responses []models.RSVPResponse, contributors models.RSVPHeadcountContributors) ([]rsvpContributorTally, int) {
	tallies := make([]rsvpContributorTally, 0, len(contributors))
	total := 0

	for _, contributor := range contributors {
		tally := rsvpContributorTally{
			Label:     contributor.Label,
			AnswerKey: contributor.AnswerKey,
			Mode:      string(contributor.Mode),
		}
		for _, response := range responses {
			got := evaluateHeadcountContributor(contributor, response.Answers, response.Attendance)
			tally.People += got.People
			if got.Matched {
				tally.Responses++
			}
			if got.NeedsReview {
				tally.NeedsReview++
			}
			if got.Unparseable {
				tally.Unparseable++
			}
		}
		total += tally.People
		tallies = append(tallies, tally)
	}

	return tallies, total
}
```

Do **not** modify `buildRSVPAttendanceBreakdown`. It keeps serving the existing Member/Spouse cards unchanged; this is additive. Removing the `spouse_attendance` literal from it is deferred to Task 8, after the frontend reads contributors.

- [ ] **Step 4: Run to verify it passes, and that nothing regressed**

```bash
go test ./internal/handlers/ -run 'TestBuildRSVP' -v
```

Expected: the three new tests PASS **and** the two pre-existing `TestBuildRSVPAttendanceBreakdown*` tests still PASS. If the latter broke, revert — the live cards depend on them.

- [ ] **Step 5: Commit**

```bash
git add internal/handlers/rsvp_tally.go internal/handlers/rsvp_tally_test.go
git commit -m "Tally configured headcount contributors and a grand total

Additive: buildRSVPAttendanceBreakdown is untouched, so the existing
Member/Spouse cards keep working for the in-flight event while the new
contributor tally and total attending become available alongside."
```

---

## Task 6: Return contributors and the total from the tally endpoint

**Files:**
- Modify: `internal/handlers/rsvp.go:480-554`
- Test: `internal/handlers/rsvp_tally_endpoint_test.go` (create; `package handlers_test`, needs `TEST_DATABASE_URL`)

**Interfaces:**
- Consumes: `buildRSVPHeadcount` (Task 5), `legacyHeadcountContributors` (Task 4).
- Produces: tally envelope gains `contributors: []rsvpContributorTally` and `total_attending: int`. All existing keys keep their meaning. Task 7 consumes them.

- [ ] **Step 1: Write the failing test**

Create `internal/handlers/rsvp_tally_endpoint_test.go`:

```go
package handlers_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/require"
)

func TestRSVPEventPersistsHeadcountContributors(t *testing.T) {
	db := testutil.SetupTestDB(t)
	org := testutil.CreateTestOrganization(t, db)

	event := models.RSVPEvent{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		Name:            "Golden Jubilee",
		Status:          models.RSVPEventStatusDraft,
		Keyword:         "JUBILEE",
		AttendanceField: "attendance",
		HeadcountContributors: models.RSVPHeadcountContributors{
			{Label: "Children", AnswerKey: "children_count", Mode: models.RSVPHeadcountModeNumeric},
		},
	}
	require.NoError(t, db.Create(&event).Error)

	var got models.RSVPEvent
	require.NoError(t, db.First(&got, "id = ?", event.ID).Error)
	require.Len(t, got.HeadcountContributors, 1)
	require.Equal(t, "children_count", got.HeadcountContributors[0].AnswerKey)
	require.Equal(t, models.RSVPHeadcountModeNumeric, got.HeadcountContributors[0].Mode)
}

func TestRSVPEventWithoutContributorsScansEmpty(t *testing.T) {
	// Mirrors the live event, created before this column existed.
	db := testutil.SetupTestDB(t)
	org := testutil.CreateTestOrganization(t, db)

	event := models.RSVPEvent{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		Name:            "Legacy",
		Status:          models.RSVPEventStatusActive,
		Keyword:         "LEGACY",
		AttendanceField: "attendance",
	}
	require.NoError(t, db.Create(&event).Error)

	var got models.RSVPEvent
	require.NoError(t, db.First(&got, "id = ?", event.ID).Error)
	require.Empty(t, got.HeadcountContributors)
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/handlers/ -run 'TestRSVPEventPersistsHeadcountContributors|TestRSVPEventWithoutContributorsScansEmpty' -v
```

Expected: FAIL to build without Task 2, or SKIP if `TEST_DATABASE_URL` is unset. **If it skips, set the variable and run it for real** — this is the task that proves the live event survives.

- [ ] **Step 3: Extend the tally endpoint**

In `internal/handlers/rsvp.go`, widen the `Select` at `:491` to include the new column, and add the contributor tally before the envelope:

```go
	if err := a.DB.Select("attendance_field", "spouse_mobile_field", "headcount_contributors").
		Where("id = ? AND organization_id = ?", eventID, orgID).First(&ev).Error; err != nil {
```

Then, after `attendanceBreakdown` is built at `:544`:

```go
	contributors := ev.HeadcountContributors
	if len(contributors) == 0 {
		contributors = legacyHeadcountContributors(ev.AttendanceField)
	}
	contributorTallies, totalAttending := buildRSVPHeadcount(responses, contributors)
```

And extend the envelope at `:546-553` with two keys, leaving every existing key untouched:

```go
		"contributors":    contributorTallies,
		"total_attending": totalAttending,
```

- [ ] **Step 4: Run to verify it passes**

```bash
TEST_DATABASE_URL=<url> go test ./internal/handlers/ -run 'TestRSVP' -v
```

Expected: PASS.

- [ ] **Step 5: Verify against live-shaped data**

With a dev DB seeded to mirror production (271 member-yes, 207 spouse-yes, no children answers), call `GET /api/rsvp/{id}/tally`.

**Expect:** `member_attendance` and `spouse_attendance` **unchanged** from today. `total_attending` = **478** (271 + 207). `contributors` shows two legacy rows. If `member_attendance` moved even by one, stop — backwards compatibility is broken.

- [ ] **Step 6: Commit**

```bash
git add internal/handlers/rsvp.go internal/handlers/rsvp_tally_endpoint_test.go
git commit -m "Return headcount contributors and total attending from the tally

Additive: every existing envelope key keeps its meaning, so the live
dashboard is unaffected. Events with no contributors fall back to the
legacy member + spouse set."
```

---

## Task 7: Dashboard — contributor cards and the total

**Files:**
- Modify: `frontend/src/views/rsvp/RSVPResultsView.vue`

**Interfaces:**
- Consumes: `contributors[]` and `total_attending` from `GET /api/rsvp/{id}/tally` (Task 6).
- Produces: none.

- [ ] **Step 1: Render Total attending**

Add a prominent card beside the existing Total / Not started / In progress / Responded row, reading `total_attending`. Label it "Total attending". On today's data it reads **478**.

- [ ] **Step 2: Render a card per contributor**

Below the existing Member/Spouse cards, render one card per entry in `contributors[]`, showing `label` and `people`. Keep the existing Member/Spouse cards as they are — they carry the Yes/No/Pending split and their click-to-filter behaviour, which the contributor cards do not replace.

- [ ] **Step 3: Surface the flags**

Where a contributor reports `unparseable > 0` or `needs_review > 0`, show a warning on that card, e.g. `2 answers need checking`. This is the promise that a family is never silently lost — it must be visible, not buried in the export.

- [ ] **Step 4: Typecheck, lint, build**

```bash
cd frontend && npm run typecheck && npm run lint && npm run build
```

Expected: clean.

- [ ] **Step 5: Verify against the live-shaped dev instance**

Load the results page. **Expect:** Member and Spouse cards read exactly as before; Total attending reads 478; no contributor card shows a flag (no children answers exist yet).

- [ ] **Step 6: Commit**

```bash
git add frontend/src/views/rsvp/RSVPResultsView.vue
git commit -m "Show total attending and a card per headcount contributor

Existing Member/Spouse cards are untouched - they carry the Yes/No/
Pending split and click-to-filter. Contributor cards flag unparseable
and implausible counts rather than hiding them."
```

---

## Task 8: Event settings — the contributor editor

**Files:**
- Modify: `frontend/src/views/rsvp/RSVPEventBuilderView.vue`
- Modify: `internal/handlers/rsvp.go` (accept the field on create/update)

**Interfaces:**
- Consumes: `models.RSVPHeadcountContributors` (Task 2).
- Produces: none.

- [ ] **Step 1: Accept the field server-side**

Find the RSVP event create/update handlers in `internal/handlers/rsvp.go` and add `headcount_contributors` to the accepted payload and the persisted columns. Follow exactly how `attendance_map` (also jsonb) is handled — read it first.

Reject on save, with a clear message:
- a contributor with an empty `answer_key`;
- a `mode` that is neither `boolean` nor `numeric`;
- a `boolean` contributor with no `match_values`;
- two contributors sharing an `answer_key`.

- [ ] **Step 2: Add the editor UI**

In the Edit RSVP form — beside "Spouse mobile field", which is the closest existing analogue — add a **"What counts toward the headcount?"** table. Each row: **Label** (text), **Question key** (text), **Counts as** (select: "1 per Yes" → `boolean`, "the number given" → `numeric`), and for boolean, **Values meaning yes** (comma-separated, defaulting to `yes, attending`). Plus add/remove/reorder.

When an event has none configured, prefill the two legacy rows (member attendance, spouse attendance) so saving is a no-op change rather than a behaviour change. **Do not auto-save this** — the live event must not be mutated by someone merely opening the settings page.

- [ ] **Step 3: Typecheck, lint, build**

```bash
cd frontend && npm run typecheck && npm run lint && npm run build
```

Expected: clean.

- [ ] **Step 4: Round-trip test by hand**

On a dev instance: add a Children row (`children_count`, numeric), save, reload. **Expect:** it persists. Remove it, save, reload. **Expect:** it is gone and the tally returns to 478.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/views/rsvp/RSVPEventBuilderView.vue internal/handlers/rsvp.go
git commit -m "Add headcount contributor editor to RSVP event settings

Prefills the legacy member + spouse rows for events with none configured
so opening the page changes nothing until saved."
```

---

## Task 9: Remove the remaining `spouse_attendance` hardcodes

Only now — with configuration in place and the frontend reading it — retire the literals. `spouse_attendance` is hardcoded in three places; Task 1 dealt with the Vue label's *symptom*, but the label branch at `RSVPResultsView.vue:121` remains.

**Files:**
- Modify: `internal/handlers/rsvp_guests.go:168-186`
- Modify: `internal/handlers/rsvp_tally.go:52`
- Modify: `frontend/src/views/rsvp/RSVPResultsView.vue:119-123`

**Interfaces:**
- Consumes: event contributors (Task 2), `legacyHeadcountContributors` (Task 4).
- Produces: none.

- [ ] **Step 1: Write the failing test for the configurable spouse key**

Append to `internal/handlers/rsvp_tally_test.go`:

```go
func TestBuildRSVPAttendanceBreakdownUsesConfiguredSpouseKey(t *testing.T) {
	// Renaming the spouse question in the flow builder must not silently zero the
	// spouse card - which is exactly what the hardcoded key at rsvp_tally.go:52 did.
	responses := []models.RSVPResponse{{
		Attendance: models.RSVPAttendanceYes,
		Answers:    models.JSONB{"partner_coming": "yes", "spouse_mobile": "919840445616"},
	}}

	got := buildRSVPAttendanceBreakdownWithKey(responses, "spouse_mobile", "partner_coming")
	if got.Spouse.Attending != 1 {
		t.Fatalf("configured spouse key must be honoured: %+v", got.Spouse)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/handlers/ -run TestBuildRSVPAttendanceBreakdownUsesConfiguredSpouseKey -v
```

Expected: FAIL to build — `undefined: buildRSVPAttendanceBreakdownWithKey`.

- [ ] **Step 3: Parameterize the spouse key**

In `rsvp_tally.go`, extract the key. Keep `buildRSVPAttendanceBreakdown` as a thin wrapper so the two existing tests stay green untouched:

```go
// buildRSVPAttendanceBreakdown preserves the original signature and defaults for
// callers that have no configured spouse key.
func buildRSVPAttendanceBreakdown(responses []models.RSVPResponse, spouseMobileField string) rsvpAttendanceBreakdown {
	return buildRSVPAttendanceBreakdownWithKey(responses, spouseMobileField, "spouse_attendance")
}

// buildRSVPAttendanceBreakdownWithKey takes the spouse attendance key explicitly.
// It was hardcoded, so renaming the question in the flow builder silently zeroed
// the spouse card with no warning.
func buildRSVPAttendanceBreakdownWithKey(responses []models.RSVPResponse, spouseMobileField, spouseAttendanceKey string) rsvpAttendanceBreakdown {
	if strings.TrimSpace(spouseMobileField) == "" {
		spouseMobileField = "spouse_mobile"
	}
	if strings.TrimSpace(spouseAttendanceKey) == "" {
		spouseAttendanceKey = "spouse_attendance"
	}
	// ... body unchanged, except line 52 becomes:
	//   spouseAnswer := normalizedRSVPAnswer(response.Answers, spouseAttendanceKey, spouseAttendanceKey+"_title")
}
```

Copy the rest of the body verbatim from the existing function — **including** the intentional double-count at `:53-61`. Do not alter it.

- [ ] **Step 4: Pass the configured key from the endpoint**

In `rsvp.go`, derive the spouse key from the event's contributors — the second boolean contributor whose key is not the attendance field — falling back to `"spouse_attendance"`. Keep it simple and explicit; if no such contributor exists, use the default.

- [ ] **Step 5: Make the guest SQL filter use it too**

`rsvp_guests.go:173` embeds the same literal in raw SQL:

```go
answerSQL := `LOWER(TRIM(COALESCE(NULLIF(rsvp_responses.answers ->> 'spouse_attendance', ''), rsvp_responses.answers ->> 'spouse_attendance_title', '')))`
```

Parameterize the key. **Bind it as a parameter — do not interpolate it into the SQL string.** It originates from user-editable configuration.

Its `pending` branch hand-mirrors the Go double-count. Keep them consistent; if you change one, change both, and re-read the note at the top of this plan first.

- [ ] **Step 6: Remove the Vue label special-case**

`RSVPResultsView.vue:119-123` labels by literal key. Drive labels from the contributor `label` where a contributor matches, falling back to the existing prettified key:

```ts
function prettyKey(k: string): string {
  const base = k.endsWith('_title') ? k.slice(0, -'_title'.length) : k
  const contributor = contributors.value.find(c => c.answer_key === base)
  if (contributor?.label) return contributor.label
  if (base === attendanceField.value) return t('rsvp.memberAttendance')
  return base.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase())
}
```

- [ ] **Step 7: Run everything**

```bash
go build ./... && go test ./internal/handlers/ -v
cd frontend && npm run typecheck && npm run lint && npm run build
```

Expected: all PASS. The two original `TestBuildRSVPAttendanceBreakdown*` tests must still pass **unmodified** — they are the contract with the live event.

- [ ] **Step 8: Verify the trap is gone**

On a dev instance: rename the spouse question in the flow builder, set the matching contributor key in event settings, submit a response. **Expect:** the spouse card counts it. Before this task it silently read zero.

- [ ] **Step 9: Commit**

```bash
git add internal/handlers/rsvp_tally.go internal/handlers/rsvp_guests.go internal/handlers/rsvp_tally_test.go frontend/src/views/rsvp/RSVPResultsView.vue
git commit -m "Drive the spouse attendance key from configuration

The literal 'spouse_attendance' was hardcoded in three places - the Go
tally, the raw SQL guest filter and the Vue label - so renaming the
question in the flow builder silently zeroed the spouse card.

buildRSVPAttendanceBreakdown keeps its signature and defaults, and the
intentional attending/pending double-count is preserved verbatim."
```

---

## Task 10: Author the children questions (no code)

This is configuration, done in the running app. It is a task because the feature is not delivered until it exists.

- [ ] **Step 1: Add the questions to the event's flow**

In the flow builder, after the spouse questions:
1. **"Are you bringing children?"** → Yes / No quick replies.
2. *(on Yes)* **"How many children?"** → quick-reply buttons `1 / 2 / 3 / 4+` or free text.

Note the answer key the builder assigns to the count question — the next step needs it.

- [ ] **Step 2: Add the contributor**

In Edit RSVP → "What counts toward the headcount?", add: Label `Children`, Question key `<key from step 1>`, Counts as `the number given`.

- [ ] **Step 3: Test end-to-end with one real number**

Message the keyword from a test phone. Answer yes, then 2. **Expect:** the Children card reads 2; Total attending rises by 2; the results grid shows a Children column with `2` on that row; the export contains it.

- [ ] **Step 4: Test the awkward answers**

Reply `two`, then on another test row reply `lots`. **Expect:** `two` → 2. `lots` → counts 0 **and** the Children card flags `1 answer needs checking`. It must not vanish.

---

## Self-Review

- **Spec coverage.** Section 2: children questions authored → Task 10; duplicate column → Task 1. Section 3: contributor config → Tasks 2, 8; boolean/numeric modes → Task 4; grand total → Tasks 5, 6, 7; numeric parsing incl. flagging → Task 3; removing the `spouse_attendance` magic string → Task 9. ✅
- **Independence.** No reference to the reminder-send-fix plan or the follow-up plan. ✅
- **Backwards compatibility.** Tasks 5, 6, 9 are explicitly additive; the two pre-existing `TestBuildRSVPAttendanceBreakdown*` tests must pass unmodified throughout; Task 6 Step 5 pins `total_attending` = 478 against live-shaped data. ✅
- **Type consistency.** `RSVPHeadcountContributor(s)`, `RSVPHeadcountMode*` defined Task 2, used 4–9. `parseHeadcountValue`, `headcountNeedsReview` defined Task 3, used Task 4. `headcountContribution`, `evaluateHeadcountContributor`, `legacyHeadcountContributors` defined Task 4, used Tasks 5, 6, 9. `rsvpContributorTally`, `buildRSVPHeadcount` defined Task 5, used Tasks 6, 7. `buildRSVPAttendanceBreakdownWithKey` defined Task 9. ✅
- **Placeholder scan.** One empty test stub was written and explicitly deleted in Task 4 Step 1 rather than left as a TODO. No TBDs remain. ✅
- **Known soft spots**, flagged for the implementer rather than guessed: how `attendance_map` is accepted on create/update (Task 8 Step 1), and the exact shape of the RSVP event create/update payload. Both say to read first.
