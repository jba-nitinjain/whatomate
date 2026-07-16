# RSVP Follow-Up Campaigns — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reach guests who have **already responded** — impossible today — to top up their record with an answer they were never asked for, without disturbing what they already told us.

**Architecture:** A generic follow-up built from three pickers: an **audience filter** (including `missing_answer(key)`, which is self-cleaning), a **template** (+ dynamic fields + header media), and a **flow** to run when they reply. The reply merges into the existing `rsvp_responses.answers` JSONB; attendance is never recomputed; the duplicate guard is bypassed for follow-up sessions only. Nothing about children appears in the code — children is one audience filter and one flow.

**Tech Stack:** Go 1.x, GORM, fastglue/fasthttp, PostgreSQL jsonb, Redis queue, Vue 3 + TypeScript.

## Global Constraints

- **Spec:** `docs/superpowers/specs/2026-07-16-rsvp-headcount-and-followup-design.md` — Section 4.
- **BLOCKING EXTERNAL DEPENDENCY.** The live send requires a **new Meta template** with a tap button (e.g. "Tell us about children"). The org's only template, `rsvp_message_1`, has buttons `Attending` / `Not Attending` — unsuitable. **All code in this plan can be written, tested and merged before that approval lands.** Only Task 8's live send waits. Do not treat the approval as a reason to delay the work.
- **Depends on** `2026-07-16-rsvp-reminder-send-fix.md` (Tasks 1, 3, 5 — validation and media staging). Build this **after** it. That plan must not gain a dependency on this one.
- **Does NOT depend on** `2026-07-16-rsvp-headcount-contributors.md`. `missing_answer` takes any key as a string; it neither knows nor cares about contributors.
- **Never overwrite an existing answer or attendance from a follow-up.** 271 members and 207 spouses have already replied about the 19/07/2026 event. Corrupting those is worse than collecting no children data at all.
- **Go tests:** stdlib `testing` + testify v1.11.1. Pure-logic tests `package handlers`; DB tests `package handlers_test` + `testutil.SetupTestDB(t)` (skips without `TEST_DATABASE_URL`).
- **Dates displayed to users must be dd/mm/yyyy.** Store/transmit ISO.

---

## Verified Facts (do not re-derive)

Read from the codebase on 2026-07-16.

| Fact | Location |
| --- | --- |
| `func (a *App) finalizeRSVPFromSession(session *models.ChatbotSession)` — no return value | `internal/handlers/rsvp_capture.go:120-194` |
| Finalize builds `answers` from all non-`_`-prefixed SessionData keys, **replacing** the row's answers | `rsvp_capture.go:143-150` |
| Finalize upserts with `Unscoped().Model(...).Where("rsvp_event_id = ? AND contact_id = ?").Updates(...)` | `rsvp_capture.go:176-179` |
| Finalize sets `answers`, `attendance`, `responded_at`, `deleted_at: nil` | `rsvp_capture.go:169-175` |
| `const rsvpEventIDKey = "_rsvp_event_id"` | `rsvp_capture.go:56` |
| `func (a *App) rsvpAlreadyResponded(event *models.RSVPEvent, phone string) bool` — the duplicate guard | `rsvp_capture.go:37` |
| `func normalizePhoneDigits(s string) string` | `rsvp_capture.go:13` |
| `func (a *App) rsvpEventForFlow(orgID, flowID uuid.UUID) *models.RSVPEvent` | `rsvp_capture.go:59` |
| `func (a *App) seedPendingRSVPResponse(orgID, event, contactID, phone, sources...)` | `rsvp_capture.go:70` |
| `func (a *App) markRSVPStarted(eventID, contactID uuid.UUID)` | `rsvp_capture.go:111` |
| Chatbot RSVP hook — resolves event, rejects duplicates/closed/not-invited, tags session, seeds row | `internal/handlers/chatbot_processor.go:868-911` |
| Incremental answer save per step | `chatbot_processor.go:1237-1239` |
| Finalize on session close | `chatbot_processor.go:1304-1305` |
| `DuplicateMessage` field on event; live value: *"Thank you! Your RSVP has been already recorded."* | `internal/models/rsvp.go:66-113` |
| `RSVPResponse{Attendance, Answers, RespondedAt, RSVPStartedAt, RepromptedAt, ...}` | `internal/models/rsvp.go:116-136` |
| Journey status SQL (`responded` / `in_progress` / `not_started`) | `internal/handlers/rsvp_guests.go:150` |
| `rsvpGuestRosterRow` | `rsvp_guests.go:126-131` |
| Reminder campaign builder (the pattern to mirror) | `internal/handlers/rsvp_reminder_campaign.go:32` |
| `func resolveRSVPReminderParams(...)` — `{{member_name}}`, `{{event_name}}`, `{{answer.<key>}}` etc. | `internal/handlers/rsvp_reminders.go:64` |
| `func (a *App) enqueueCampaignRecipients(ctx, campaign, recipients, now, fallbackStatus) error` | `internal/handlers/campaigns.go:648-696` |
| `const CampaignSourceRSVPReminder = "rsvp_reminder"` | `internal/models/bulk.go:9` |
| RSVP routes registered here | `cmd/whatomate/main.go:721-746` |
| Results page action buttons | `frontend/src/views/rsvp/RSVPResultsView.vue` |

### The gap this closes

| Existing action | Audience | Reaches responders? |
| --- | --- | --- |
| Send invitations | First contact | No |
| Reminders | Never started (`loadNotStartedRSVPGuests`) | **No** |
| Re-prompt pending | Pending / mid-flow | **No** |
| Keyword | Blocked by `rsvpAlreadyResponded` → `DuplicateMessage` | **Actively refused** |

---

## File Structure

| File | Responsibility | Change |
| --- | --- | --- |
| `internal/models/rsvp.go` | RSVP schema | Add `RSVPFollowUp` model + `RSVPFollowUpAudience` constants |
| `internal/handlers/rsvp_followup_audience.go` | Audience filter → SQL | Create |
| `internal/handlers/rsvp_followup_audience_test.go` | Filter tests | Create |
| `internal/handlers/rsvp_followup.go` | Preview + send endpoints | Create |
| `internal/handlers/rsvp_followup_test.go` | Endpoint tests | Create |
| `internal/handlers/rsvp_followup_campaign.go` | Campaign build + dispatch | Create |
| `internal/handlers/rsvp_capture.go` | Session finalize | Modify: merge semantics for follow-ups |
| `internal/handlers/rsvp_capture_followup_test.go` | Merge tests | Create |
| `internal/handlers/chatbot_processor.go` | Flow hook | Modify: bypass duplicate guard for follow-ups |
| `cmd/whatomate/main.go` | Routes | Modify: register follow-up routes |
| `frontend/src/components/rsvp/RSVPFollowUpDialog.vue` | Follow-up UI | Create |
| `frontend/src/views/rsvp/RSVPResultsView.vue` | Results page | Modify: Follow-up button |

---

## Task 1: Audience filters

`missing_answer` is the primary filter and the reason this design generalizes: it is **self-cleaning**. Respondents drop out as answers arrive, so re-sending chases only whoever is still missing — no lists to maintain, no double-messaging.

**Files:**
- Create: `internal/handlers/rsvp_followup_audience.go`
- Test: `internal/handlers/rsvp_followup_audience_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  ```go
  type RSVPFollowUpAudience string
  const (
      RSVPFollowUpAudienceNotStarted    RSVPFollowUpAudience = "not_started"
      RSVPFollowUpAudienceRespondedYes  RSVPFollowUpAudience = "responded_yes"
      RSVPFollowUpAudienceRespondedNo   RSVPFollowUpAudience = "responded_no"
      RSVPFollowUpAudienceMissingAnswer RSVPFollowUpAudience = "missing_answer"
  )
  func rsvpFollowUpAudienceClause(audience RSVPFollowUpAudience, answerKey string) (sql string, args []interface{}, err error)
  ```
  Tasks 2 and 3 call it.

- [ ] **Step 1: Write the failing test**

Create `internal/handlers/rsvp_followup_audience_test.go`:

```go
package handlers

import (
	"strings"
	"testing"
)

func TestRSVPFollowUpAudienceMissingAnswer(t *testing.T) {
	sql, args, err := rsvpFollowUpAudienceClause(RSVPFollowUpAudienceMissingAnswer, "children_count")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The key comes from user configuration and must never be interpolated.
	if strings.Contains(sql, "children_count") {
		t.Fatalf("answer key must be bound as a parameter, not interpolated into SQL: %s", sql)
	}
	if len(args) != 1 || args[0] != "children_count" {
		t.Fatalf("expected the key bound as the only arg, got %+v", args)
	}
	// Must only chase people who actually replied - chasing non-responders for a
	// follow-up answer is what Reminders is for.
	if !strings.Contains(sql, "responded_at IS NOT NULL") {
		t.Fatalf("missing_answer must be scoped to responders: %s", sql)
	}
}

func TestRSVPFollowUpAudienceMissingAnswerTreatsEmptyAsMissing(t *testing.T) {
	sql, _, err := rsvpFollowUpAudienceClause(RSVPFollowUpAudienceMissingAnswer, "children_count")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// A key present but empty ("") is missing, not answered.
	if !strings.Contains(sql, "IS NULL") || !strings.Contains(sql, "''") {
		t.Fatalf("empty string must count as missing: %s", sql)
	}
}

func TestRSVPFollowUpAudienceMissingAnswerRequiresKey(t *testing.T) {
	if _, _, err := rsvpFollowUpAudienceClause(RSVPFollowUpAudienceMissingAnswer, ""); err == nil {
		t.Fatal("missing_answer without a key must be rejected")
	}
	if _, _, err := rsvpFollowUpAudienceClause(RSVPFollowUpAudienceMissingAnswer, "   "); err == nil {
		t.Fatal("whitespace-only key must be rejected")
	}
}

func TestRSVPFollowUpAudienceResponded(t *testing.T) {
	yes, args, err := rsvpFollowUpAudienceClause(RSVPFollowUpAudienceRespondedYes, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(yes, "attendance") || len(args) != 1 || args[0] != "yes" {
		t.Fatalf("responded_yes must filter attendance = yes, got %s / %+v", yes, args)
	}

	no, args, err := rsvpFollowUpAudienceClause(RSVPFollowUpAudienceRespondedNo, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 1 || args[0] != "no" {
		t.Fatalf("responded_no must filter attendance = no, got %s / %+v", no, args)
	}
}

func TestRSVPFollowUpAudienceNotStarted(t *testing.T) {
	sql, _, err := rsvpFollowUpAudienceClause(RSVPFollowUpAudienceNotStarted, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(sql, "rsvp_started_at IS NULL") {
		t.Fatalf("not_started must match the existing journey definition (rsvp_guests.go:150): %s", sql)
	}
}

func TestRSVPFollowUpAudienceRejectsUnknown(t *testing.T) {
	if _, _, err := rsvpFollowUpAudienceClause(RSVPFollowUpAudience("everyone"), ""); err == nil {
		t.Fatal("unknown audience must be rejected, not silently matched")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/handlers/ -run TestRSVPFollowUpAudience -v
```

Expected: FAIL to build — undefined.

- [ ] **Step 3: Implement**

Create `internal/handlers/rsvp_followup_audience.go`:

```go
package handlers

import (
	"fmt"
	"strings"
)

// RSVPFollowUpAudience selects which guests a follow-up targets.
type RSVPFollowUpAudience string

const (
	// RSVPFollowUpAudienceNotStarted matches guests who never began the RSVP.
	// Mirrors the journey definition at rsvp_guests.go:150.
	RSVPFollowUpAudienceNotStarted RSVPFollowUpAudience = "not_started"
	// RSVPFollowUpAudienceRespondedYes matches guests attending.
	RSVPFollowUpAudienceRespondedYes RSVPFollowUpAudience = "responded_yes"
	// RSVPFollowUpAudienceRespondedNo matches guests not attending.
	RSVPFollowUpAudienceRespondedNo RSVPFollowUpAudience = "responded_no"
	// RSVPFollowUpAudienceMissingAnswer matches guests who replied but never
	// answered a given question. Self-cleaning: as answers arrive, the audience
	// shrinks, so re-sending chases only whoever is still missing.
	RSVPFollowUpAudienceMissingAnswer RSVPFollowUpAudience = "missing_answer"
)

// rsvpFollowUpAudienceClause returns a WHERE fragment and its bind args for an
// audience. The answer key originates from user configuration and is always bound,
// never interpolated.
func rsvpFollowUpAudienceClause(audience RSVPFollowUpAudience, answerKey string) (string, []interface{}, error) {
	switch audience {
	case RSVPFollowUpAudienceNotStarted:
		return "rsvp_responses.rsvp_started_at IS NULL AND rsvp_responses.responded_at IS NULL", nil, nil

	case RSVPFollowUpAudienceRespondedYes:
		return "rsvp_responses.responded_at IS NOT NULL AND rsvp_responses.attendance = ?",
			[]interface{}{"yes"}, nil

	case RSVPFollowUpAudienceRespondedNo:
		return "rsvp_responses.responded_at IS NOT NULL AND rsvp_responses.attendance = ?",
			[]interface{}{"no"}, nil

	case RSVPFollowUpAudienceMissingAnswer:
		key := strings.TrimSpace(answerKey)
		if key == "" {
			return "", nil, fmt.Errorf("missing_answer requires an answer key")
		}
		// Scoped to responders: chasing someone who never replied is what Reminders
		// is for. A key that is absent OR empty counts as missing.
		return `rsvp_responses.responded_at IS NOT NULL
			AND COALESCE(NULLIF(rsvp_responses.answers ->> ?, ''), '') = ''`,
			[]interface{}{key}, nil

	default:
		return "", nil, fmt.Errorf("unknown follow-up audience: %q", audience)
	}
}
```

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/handlers/ -run TestRSVPFollowUpAudience -v
```

Expected: all six PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/handlers/rsvp_followup_audience.go internal/handlers/rsvp_followup_audience_test.go
git commit -m "Add RSVP follow-up audience filters

missing_answer is the primary filter and is self-cleaning: respondents
drop out as answers arrive, so re-sending chases only whoever is still
missing. The answer key is always bound, never interpolated."
```

---

## Task 2: Merge follow-up answers without clobbering the record

The dangerous task. `finalizeRSVPFromSession` (`rsvp_capture.go:143-179`) **replaces** the whole `answers` map and rewrites `attendance` on every completed session. Run a follow-up flow through it unchanged and 271 members lose their attendance and spouse answers.

**Files:**
- Modify: `internal/handlers/rsvp_capture.go`
- Test: `internal/handlers/rsvp_capture_followup_test.go` (create)

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `const rsvpFollowUpKey = "_rsvp_followup"` — session flag, `_`-prefixed so the existing filter at `rsvp_capture.go:145` keeps it out of stored answers.
  - `func mergeRSVPAnswers(existing, incoming models.JSONB) models.JSONB` — incoming wins per key; existing keys absent from incoming survive.

  Tasks 3 and 5 set the flag.

- [ ] **Step 1: Write the failing test**

Create `internal/handlers/rsvp_capture_followup_test.go`:

```go
package handlers

import (
	"testing"

	"github.com/nikyjain/whatomate/internal/models"
)

func TestMergeRSVPAnswersKeepsExisting(t *testing.T) {
	// The live event has 271 members who already answered attendance and spouse.
	// A follow-up asking only about children must not erase any of it.
	existing := models.JSONB{
		"attendance":              "yes",
		"attendance_title":        "Attending",
		"spouse_attendance":       "yes",
		"spouse_attendance_title": "Attending",
		"spouse_mobile":           "919840026019",
	}
	incoming := models.JSONB{
		"children_count": "2",
	}

	got := mergeRSVPAnswers(existing, incoming)

	for k, want := range existing {
		if got[k] != want {
			t.Errorf("follow-up erased %q: got %v, want %v", k, got[k], want)
		}
	}
	if got["children_count"] != "2" {
		t.Errorf("children_count = %v, want 2", got["children_count"])
	}
	if len(got) != 6 {
		t.Errorf("expected 6 keys, got %d: %+v", len(got), got)
	}
}

func TestMergeRSVPAnswersIncomingWins(t *testing.T) {
	// A guest correcting an earlier answer must be honoured.
	existing := models.JSONB{"children_count": "2"}
	incoming := models.JSONB{"children_count": "3"}

	got := mergeRSVPAnswers(existing, incoming)
	if got["children_count"] != "3" {
		t.Errorf("incoming must win: got %v, want 3", got["children_count"])
	}
}

func TestMergeRSVPAnswersHandlesNil(t *testing.T) {
	got := mergeRSVPAnswers(nil, models.JSONB{"a": "1"})
	if got["a"] != "1" {
		t.Errorf("nil existing must yield incoming: %+v", got)
	}

	got = mergeRSVPAnswers(models.JSONB{"a": "1"}, nil)
	if got["a"] != "1" {
		t.Errorf("nil incoming must preserve existing: %+v", got)
	}

	if got := mergeRSVPAnswers(nil, nil); len(got) != 0 {
		t.Errorf("nil/nil must yield empty, got %+v", got)
	}
}

func TestMergeRSVPAnswersDoesNotAliasExisting(t *testing.T) {
	existing := models.JSONB{"a": "1"}
	incoming := models.JSONB{"b": "2"}

	got := mergeRSVPAnswers(existing, incoming)
	got["c"] = "3"

	if _, leaked := existing["c"]; leaked {
		t.Error("merge must not mutate the existing map")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/handlers/ -run TestMergeRSVPAnswers -v
```

Expected: FAIL to build — `undefined: mergeRSVPAnswers`.

- [ ] **Step 3: Implement the merge**

Add to `internal/handlers/rsvp_capture.go`:

```go
// rsvpFollowUpKey marks a session as a follow-up: it tops up an existing response
// rather than making a new one. The "_" prefix keeps it out of stored answers via
// the existing filter in finalizeRSVPFromSession.
const rsvpFollowUpKey = "_rsvp_followup"

// mergeRSVPAnswers overlays incoming answers onto existing ones. Incoming wins per
// key so a guest can correct themselves; keys absent from incoming survive
// untouched. Returns a new map - neither input is mutated.
func mergeRSVPAnswers(existing, incoming models.JSONB) models.JSONB {
	merged := models.JSONB{}
	for k, v := range existing {
		merged[k] = v
	}
	for k, v := range incoming {
		merged[k] = v
	}
	return merged
}
```

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/handlers/ -run TestMergeRSVPAnswers -v
```

Expected: all four PASS.

- [ ] **Step 5: Write the failing test for finalize's follow-up branch**

Append to `internal/handlers/rsvp_capture_followup_test.go`. This is a DB test — move it to a `_test.go` file in `package handlers_test` if `finalizeRSVPFromSession` is unexported and unreachable; the repo already exposes `FinalizeRSVPFromSessionForTest` (`rsvp_capture.go`) for exactly this, so prefer that.

```go
func TestFinalizeFollowUpPreservesAttendanceAndAnswers(t *testing.T) {
	db := testutil.SetupTestDB(t)
	// Arrange: a guest who already responded yes, with spouse answers.
	// Act: run a follow-up session carrying only children_count.
	// Assert:
	//   - attendance is still "yes" (NOT recomputed to pending)
	//   - spouse_attendance and spouse_mobile survive
	//   - children_count is added
	//   - responded_at is NOT moved (they responded days ago)
	//
	// Build this using testutil.CreateTestOrganization + models.RSVPEvent +
	// models.RSVPResponse fixtures, mirroring rsvp_test.go's setup, then call
	// app.FinalizeRSVPFromSessionForTest with SessionData containing
	// rsvpEventIDKey, rsvpFollowUpKey and children_count.
	_ = db
	t.Skip("implement alongside Step 6 - see rsvp_test.go for the fixture pattern")
}
```

Replace the `t.Skip` with the real test in Step 6. **Do not leave it skipped** — a skipped test on the one path that can corrupt 271 live records is worse than no test, because it reads as covered.

- [ ] **Step 6: Add the follow-up branch to finalize**

In `finalizeRSVPFromSession` (`rsvp_capture.go:120-194`), after the answers map is built (`:150`) and before the updates map (`:169`):

```go
	isFollowUp, _ := session.SessionData[rsvpFollowUpKey].(bool)

	updates := map[string]interface{}{}
	if isFollowUp {
		// A follow-up tops up an existing response. It must never recompute
		// attendance or move responded_at: the guest answered days ago and only
		// told us one extra thing now.
		var current models.RSVPResponse
		if err := a.DB.Where("rsvp_event_id = ? AND contact_id = ?", event.ID, session.ContactID).
			First(&current).Error; err != nil {
			a.Log.Warn("RSVP follow-up has no existing response; ignoring",
				"event_id", event.ID, "contact_id", session.ContactID)
			return
		}
		updates["answers"] = mergeRSVPAnswers(current.Answers, answers)
	} else {
		updates["answers"] = answers
		updates["attendance"] = attendance
		updates["responded_at"] = now
		updates["deleted_at"] = nil
	}
```

Then leave the existing upsert at `:176-179` to apply `updates`. **The create-if-missing fallback at `:180-192` must not run for a follow-up** — a follow-up with no existing row is a bug, not a new guest. Guard it:

```go
	if res.Error == nil && res.RowsAffected == 0 && !isFollowUp {
		// ... existing Create(&models.RSVPResponse{...}) unchanged
	}
```

Read the real code around `:169-192` before editing — the variable names above (`answers`, `attendance`, `now`, `res`) are taken from the current source, but confirm.

- [ ] **Step 7: Write the real test and run it**

Replace Step 5's skipped stub with a working test, then:

```bash
TEST_DATABASE_URL=<url> go test ./internal/handlers/ -run 'TestFinalizeFollowUp|TestMergeRSVPAnswers' -v
```

Expected: PASS. **Do not proceed while this skips.**

- [ ] **Step 8: Confirm the normal path is untouched**

```bash
TEST_DATABASE_URL=<url> go test ./internal/handlers/ -run 'TestRSVP' -v
```

Expected: PASS — every existing RSVP test, unmodified.

- [ ] **Step 9: Commit**

```bash
git add internal/handlers/rsvp_capture.go internal/handlers/rsvp_capture_followup_test.go
git commit -m "Merge follow-up answers instead of replacing the response

finalizeRSVPFromSession replaced the whole answers map and rewrote
attendance on every completed session. A follow-up asking only about
children would have erased attendance and spouse answers for everyone who
already replied.

Follow-up sessions now merge into the existing answers, leave attendance
and responded_at alone, and never create a row."
```

---

## Task 3: Let follow-ups past the duplicate guard

`chatbot_processor.go:868-911` rejects a responder via `rsvpAlreadyResponded` → `DuplicateMessage`. That protection must stay for the main RSVP and must not apply to follow-ups.

**Files:**
- Modify: `internal/handlers/chatbot_processor.go:868-911`
- Test: `internal/handlers/rsvp_followup_guard_test.go` (create)

**Interfaces:**
- Consumes: `rsvpFollowUpKey` (Task 2).
- Produces: `func rsvpShouldBlockDuplicate(isFollowUp, alreadyResponded bool) bool`.

- [ ] **Step 1: Write the failing test**

Create `internal/handlers/rsvp_followup_guard_test.go`:

```go
package handlers

import "testing"

func TestRSVPShouldBlockDuplicate(t *testing.T) {
	cases := []struct {
		name                       string
		isFollowUp, alreadyReplied bool
		want                       bool
	}{
		{"main rsvp, first time", false, false, false},
		{"main rsvp, already replied - still blocked", false, true, true},
		{"follow-up, already replied - allowed through", true, true, false},
		{"follow-up, never replied", true, false, false},
	}
	for _, c := range cases {
		if got := rsvpShouldBlockDuplicate(c.isFollowUp, c.alreadyReplied); got != c.want {
			t.Errorf("%s: rsvpShouldBlockDuplicate(%v, %v) = %v, want %v",
				c.name, c.isFollowUp, c.alreadyReplied, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/handlers/ -run TestRSVPShouldBlockDuplicate -v
```

Expected: FAIL to build — undefined.

- [ ] **Step 3: Implement**

Add to `internal/handlers/rsvp_capture.go`:

```go
// rsvpShouldBlockDuplicate decides whether to turn a sender away with the event's
// DuplicateMessage. A follow-up deliberately targets people who already responded,
// so the guard must not apply to it - but it still protects the main RSVP.
func rsvpShouldBlockDuplicate(isFollowUp, alreadyResponded bool) bool {
	return !isFollowUp && alreadyResponded
}
```

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/handlers/ -run TestRSVPShouldBlockDuplicate -v
```

Expected: PASS.

- [ ] **Step 5: Wire it into the chatbot hook**

In `chatbot_processor.go:868-911`, find the `rsvpAlreadyResponded` check and route it through the helper. The session must already be tagged as a follow-up at that point — Task 5 tags it when the guest taps the follow-up template's button. Read the surrounding code and adapt:

```go
	isFollowUp, _ := session.SessionData[rsvpFollowUpKey].(bool)
	if rsvpShouldBlockDuplicate(isFollowUp, a.rsvpAlreadyResponded(event, phone)) {
		// ... existing DuplicateMessage send, unchanged
	}
```

Also ensure the **not-invited** and **closed-event** checks still apply to follow-ups — a follow-up is not a licence to bypass those. Only the duplicate guard changes.

- [ ] **Step 6: Verify**

```bash
go build ./... && TEST_DATABASE_URL=<url> go test ./internal/handlers/ -run 'TestRSVP|TestChatbot' -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/handlers/rsvp_capture.go internal/handlers/chatbot_processor.go internal/handlers/rsvp_followup_guard_test.go
git commit -m "Allow follow-up sessions past the RSVP duplicate guard

A follow-up deliberately targets guests who already responded, who are
otherwise turned away with DuplicateMessage. The guard still protects the
main RSVP; not-invited and closed-event checks are unchanged."
```

---

## Task 4: Follow-up preview

**Files:**
- Create: `internal/handlers/rsvp_followup.go`
- Modify: `cmd/whatomate/main.go`
- Test: `internal/handlers/rsvp_followup_test.go`

**Interfaces:**
- Consumes: `rsvpFollowUpAudienceClause` (Task 1).
- Produces:
  - `GET /api/rsvp/{id}/followup/preview?audience=<a>&answer_key=<k>` → `(a *App) PreviewRSVPFollowUp(r *fastglue.Request) error`, returning `{"eligible": n, "skipped": [...], "recipients": [...]}`.
  - `func (a *App) loadRSVPFollowUpGuests(orgID, eventID uuid.UUID, audience RSVPFollowUpAudience, answerKey string) ([]rsvpGuestRosterRow, error)`

  Task 5 reuses the loader.

- [ ] **Step 1: Write the failing test**

Create `internal/handlers/rsvp_followup_test.go` (`package handlers_test`, DB-backed). Seed one event with four responses: (a) responded yes, no `children_count`; (b) responded yes, `children_count: "2"`; (c) responded yes, `children_count: ""`; (d) never started. Then assert `loadRSVPFollowUpGuests(..., RSVPFollowUpAudienceMissingAnswer, "children_count")` returns **exactly (a) and (c)** — (b) has answered, (d) never replied.

Mirror the fixture setup in `rsvp_test.go`. Write the test fully; do not skip it.

- [ ] **Step 2: Run to verify it fails**

```bash
TEST_DATABASE_URL=<url> go test ./internal/handlers/ -run TestLoadRSVPFollowUpGuests -v
```

Expected: FAIL to build — undefined.

- [ ] **Step 3: Implement the loader and the preview handler**

Create `internal/handlers/rsvp_followup.go`. Mirror `RSVPReminderPreview` (`rsvp_reminders.go:146`) and the roster query in `ListRSVPGuests` (`rsvp_guests.go:133`) — read both and follow their shape rather than inventing a third convention.

The loader applies the audience clause plus the org/event scope and preloads `Contact`. The preview handler reuses `rsvpReminderSkipReason` from the reminder-fix plan (Task 4 there) if it is available, so preview and send agree; if that plan has not landed, inline the same predicate rather than counting nil-contact rows as eligible — repeating the reminder bug here would be inexcusable.

- [ ] **Step 4: Register the route**

In `cmd/whatomate/main.go`, beside the RSVP routes at `:721-746`:

```go
	g.GET("/api/rsvp/{id}/followup/preview", app.PreviewRSVPFollowUp)
```

Copy the auth/permission wrapper from the neighbouring RSVP routes exactly.

- [ ] **Step 5: Run to verify it passes**

```bash
TEST_DATABASE_URL=<url> go test ./internal/handlers/ -run TestLoadRSVPFollowUpGuests -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/handlers/rsvp_followup.go internal/handlers/rsvp_followup_test.go cmd/whatomate/main.go
git commit -m "Add RSVP follow-up preview

Loads guests by audience filter and reports who is eligible and who will
be skipped and why, so preview and send cannot drift apart."
```

---

## Task 5: Follow-up send

**Files:**
- Create: `internal/handlers/rsvp_followup_campaign.go`
- Modify: `cmd/whatomate/main.go`
- Modify: `internal/models/bulk.go` (source type constant)

**Interfaces:**
- Consumes: `loadRSVPFollowUpGuests` (Task 4); `validateRSVPReminderCampaignSendable` and `rsvpReminderStagingKey` from the reminder-fix plan; `enqueueCampaignRecipients` (`campaigns.go:648`); `resolveRSVPReminderParams` (`rsvp_reminders.go:64`).
- Produces:
  - `const CampaignSourceRSVPFollowUp = "rsvp_followup"` in `internal/models/bulk.go`, beside `CampaignSourceRSVPReminder` at `:9`.
  - `POST /api/rsvp/{id}/followup/send` → `(a *App) SendRSVPFollowUp(r *fastglue.Request) error`.

- [ ] **Step 1: Add the source constant**

In `internal/models/bulk.go`, beside `:9`:

```go
const CampaignSourceRSVPFollowUp = "rsvp_followup"
```

- [ ] **Step 2: Implement the send handler**

Create `internal/handlers/rsvp_followup_campaign.go`, mirroring `createRSVPReminderCampaign` (`rsvp_reminder_campaign.go:32`) closely. Read that file first; this is a variation on it, not a new invention.

Request body: `{audience, answer_key, flow_id, template_id, template_params, staging_id, staging_filename, staging_mime_type, response_ids?}`.

Sequence — **order matters**:
1. Resolve org + event; 404 if absent.
2. Validate audience via `rsvpFollowUpAudienceClause`; 400 on error.
3. Resolve and require an APPROVED template.
4. Resolve the flow; require it belongs to the org. **Reject the event's own primary `flow_id`** — running the main RSVP flow as a follow-up would re-ask attendance and, via Task 2's merge, produce a confusing half-update.
5. Load guests via `loadRSVPFollowUpGuests`.
6. Build the campaign with `SourceType: models.CampaignSourceRSVPFollowUp`, `SourceID: &event.ID`.
7. Attach staged header media (same as the reminder path).
8. **Validate before enqueue** — `validateRSVPReminderCampaignSendable`. This is the whole lesson of the 15/07 failure; do not skip it here.
9. Persist campaign + recipients in one tx.
10. Enqueue.

Record the chosen `flow_id` against the campaign so the chatbot hook (Task 6) knows which flow to run when the guest taps through.

- [ ] **Step 3: Register the route**

```go
	g.POST("/api/rsvp/{id}/followup/send", app.SendRSVPFollowUp)
```

- [ ] **Step 4: Verify**

```bash
go build ./... && TEST_DATABASE_URL=<url> go test ./internal/handlers/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/handlers/rsvp_followup_campaign.go internal/models/bulk.go cmd/whatomate/main.go
git commit -m "Add RSVP follow-up send

Mirrors the reminder campaign path, including validating header media
before enqueue. Refuses the event's primary flow as a follow-up flow,
which would re-ask attendance."
```

---

## Task 6: Tag the follow-up session when the guest replies

**Files:**
- Modify: `internal/handlers/chatbot_processor.go:868-911`

**Interfaces:**
- Consumes: `rsvpFollowUpKey` (Task 2), `CampaignSourceRSVPFollowUp` (Task 5).
- Produces: none.

- [ ] **Step 1: Set the flag when the session starts from a follow-up**

When a guest taps the follow-up template's button, the resulting session must carry `rsvpFollowUpKey: true` **before** the duplicate guard runs (Task 3) and before finalize (Task 2). Set it where the session's SessionData is first populated in the RSVP hook, keyed off the follow-up campaign the inbound message replies to.

Read `chatbot_processor.go:868-911` to find where `_rsvp_event_id` is set (`rsvpEventIDKey`) and set the follow-up flag in the same place, from the same resolution.

- [ ] **Step 2: Verify the flag survives a multi-step flow**

The flag must persist across every step of the follow-up flow, not just the first. Confirm SessionData is carried between steps (`chatbot_processor.go:1237-1239` saves incrementally) — if it is rebuilt rather than carried, the flag must be re-derived each step, or Task 2's merge silently degrades into a destructive replace on the final step.

**This is the highest-risk step in the plan.** Verify it explicitly; do not assume.

- [ ] **Step 3: Verify**

```bash
go build ./... && TEST_DATABASE_URL=<url> go test ./internal/handlers/ -run 'TestRSVP|TestChatbot' -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/handlers/chatbot_processor.go
git commit -m "Tag follow-up chatbot sessions

The flag must be set before the duplicate guard and survive every step of
the flow, or the final step would replace the response instead of merging
into it."
```

---

## Task 7: Follow-up dialog

**Files:**
- Create: `frontend/src/components/rsvp/RSVPFollowUpDialog.vue`
- Modify: `frontend/src/views/rsvp/RSVPResultsView.vue`

**Interfaces:**
- Consumes: preview (Task 4), send (Task 5).
- Produces: none.

- [ ] **Step 1: Add the Follow-up button**

On the Results page, beside **Reminders** and **Re-prompt pending**.

- [ ] **Step 2: Build the dialog**

Model it on `RSVPReminderDialog.vue` — read it and follow its structure. Three pickers:

1. **Who** — audience select: *Hasn't answered a question yet* (default) with a question-key input; *Not started*; *Responded yes*; *Responded no*.
2. **What to send** — template select, dynamic-field mapper, header-media upload shown only when the chosen template needs one (reuse the reminder dialog's control from the reminder-fix plan).
3. **What to ask** — flow select, excluding the event's primary flow.

- [ ] **Step 3: Show the live audience count**

Call the preview as the audience changes. Show e.g. `271 guests will be asked`. Make it plain this shrinks by itself: *"Guests drop off this list as they answer, so you can safely send again later."*

- [ ] **Step 4: Block and explain**

Disable send while: no template; no flow; the template needs media and none is attached; the audience is `missing_answer` with no key. Show the reason inline.

- [ ] **Step 5: Report honestly**

If the send reports `sent == 0 && failed > 0`, show an error with the first recipient's `error_message`. Same rule as the reminder dialog, same reason.

- [ ] **Step 6: Typecheck, lint, build, commit**

```bash
cd frontend && npm run typecheck && npm run lint && npm run build
cd .. && git add frontend/src/components/rsvp/RSVPFollowUpDialog.vue frontend/src/views/rsvp/RSVPResultsView.vue
git commit -m "Add RSVP follow-up dialog

Audience, template and flow pickers, with a live audience count and an
explanation that the missing-answer audience shrinks by itself."
```

---

## Task 8: End-to-end verification

**Gated on Meta approving the follow-up template.** Everything above ships without it.

- [ ] **Step 1: Full suite**

```bash
go build ./... && TEST_DATABASE_URL=<url> make test
cd frontend && npm run typecheck && npm run build
```

Expected: clean.

- [ ] **Step 2: The test that matters — a real top-up**

On a dev instance, with a guest who has already responded yes with a spouse:

1. Record their **exact** current row: attendance, every answer key, `responded_at`.
2. Send a follow-up to `missing_answer(children_count)` targeting only them.
3. Tap through. Answer: yes, 2.
4. **Expect:** `children_count = 2` added. Attendance still `yes`. Spouse answers **byte-identical**. `responded_at` **unchanged**.

If any prior answer moved, **stop and revert**. This path can corrupt 271 live records.

- [ ] **Step 3: Confirm the audience self-cleans**

Re-run the preview. **Expect:** that guest is gone from the audience. Send again. **Expect:** they are not messaged twice.

- [ ] **Step 4: Confirm the duplicate guard still guards**

From the same test number, message the event **keyword**. **Expect:** *"Thank you! Your RSVP has been already recorded."* — the main RSVP is still protected.

- [ ] **Step 5: Confirm a media-header follow-up is refused without media**

Force a send with a media-header template and no `staging_id`. **Expect:** 4xx naming the missing media; no campaign row; nothing queued.

- [ ] **Step 6: Commit any fixes**

Do not mark this plan complete on a green unit suite. The unit tests passed for the reminder code that failed 1,008 times.

---

## Out of scope

- Age bands, named children, per-child records (spec: Out of scope).
- Follow-up answers counting toward the duplicate guard.
- Scheduling a follow-up for later — send-now only. Add it when asked.
- Reworking `Send invitations`.
- The deferred reminder-path items listed in the reminder-fix plan.

---

## Self-Review

- **Spec coverage.** Audience filters incl. `missing_answer` → Task 1; template picker + dynamic fields + media → Tasks 5, 7; flow picker → Tasks 5, 7; answer-merge semantics → Task 2; duplicate-guard bypass → Task 3; new Meta template dependency → called out in Global Constraints and Task 8. ✅
- **Dependency direction.** Depends on the reminder-fix plan; nothing here is depended on by it. Independent of the headcount plan. ✅
- **Type consistency.** `RSVPFollowUpAudience*` and `rsvpFollowUpAudienceClause` defined Task 1, used 4, 5. `rsvpFollowUpKey`, `mergeRSVPAnswers` defined Task 2, used 3, 6. `rsvpShouldBlockDuplicate` defined Task 3, used Task 3. `loadRSVPFollowUpGuests` defined Task 4, used Task 5. `CampaignSourceRSVPFollowUp` defined Task 5. ✅
- **Placeholder scan.** Task 2 Step 5 deliberately writes a skipped stub and Step 7 requires replacing it before proceeding — called out explicitly rather than left as a silent TODO. No other placeholders. ✅
- **Highest risks, flagged inline:** Task 2 (finalize replaces rather than merges — can erase 271 live records) and Task 6 Step 2 (the follow-up flag must survive every flow step, or the last step turns the merge back into a destructive replace).
