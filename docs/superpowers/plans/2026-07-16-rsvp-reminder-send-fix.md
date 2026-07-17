# RSVP Reminder Send Fix — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make RSVP reminders actually send, by attaching the template's header media, validating before dispatch instead of failing 1,008 times in silence, and reporting failure honestly.

**Architecture:** Three layers. (1) Fix `validateCampaignReadyForStart` so it checks the same field the worker actually sends (`HeaderMediaLocalPath`) — a shared-path change affecting four call sites. (2) Add a staging endpoint so the RSVP reminder dialog can upload header media before the campaign exists, then attach it during campaign creation and validate before enqueue. (3) Surface skipped guests and all-failed campaigns instead of swallowing them.

**Tech Stack:** Go 1.x, GORM, fastglue/fasthttp, PostgreSQL (jsonb), Redis queue, Vue 3 + TypeScript.

## Global Constraints

- **This plan must remain independently deployable.** Do NOT introduce any code dependency on the headcount-contributors work (Section 3) or follow-up campaigns (Section 4). No shared new types with those plans.
- **Spec:** `docs/superpowers/specs/2026-07-16-rsvp-headcount-and-followup-design.md` — Section 1 only.
- **Go tests:** stdlib `testing` + `github.com/stretchr/testify` v1.11.1 (`require`). Run with `go test ./internal/handlers/ -run <Name> -v`.
- **DB-backed tests** (`package handlers_test`) call `testutil.SetupTestDB(t)`, which **skips** unless `TEST_DATABASE_URL` is set. Pure-logic tests (`package handlers`) need no DB — prefer them where possible.
- **Dates displayed to users must be dd/mm/yyyy.** Store/transmit ISO.
- **Do not** reformat or restructure files beyond the changes described. Follow surrounding style; the repo uses plain `fmt.Errorf`, no sentinel errors.
- **Media storage:** `saveCampaignMedia(id string, data []byte, mimeType string) (string, error)` returns a **relative** path (`campaigns/<id><ext>`), not absolute.

---

## Verified Facts (do not re-derive)

These were read from the live codebase on 2026-07-16. Trust them; re-check only if a build fails.

| Fact | Location |
| --- | --- |
| `func (a *App) validateCampaignReadyForStart(campaign *models.BulkMessageCampaign) error` | `internal/handlers/campaigns.go:620-640` |
| Validator checks only `HeaderMediaID` / `HeaderMediaURL` — **never** `HeaderMediaLocalPath` | `campaigns.go:633-637` |
| Worker sends `HeaderMediaLocalPath` in preference to URL | `internal/worker/worker.go:144-145` |
| Existing tri-field emptiness check to mirror | `campaigns.go:1841` |
| Validator call sites (4) | `campaigns.go:577`, `campaigns.go:972`, `campaigns.go:1088`, `internal/handlers/campaign_scheduler.go:61` |
| `func (a *App) populateCampaignHeaderMediaFromURL(ctx, campaign) error` — in-memory only, does not persist | `campaigns.go:427-448` |
| `func (a *App) saveCampaignMedia(campaignID string, data []byte, mimeType string) (string, error)` | `campaigns.go:1701` |
| `func detectCampaignMediaMimeType(filename, contentType string, data []byte) string` | used at `campaigns.go:1651` |
| `UploadCampaignMedia` — multipart, 16MB cap (`const maxMediaSize = 16 << 20`) | `campaigns.go:1591-1698` |
| `func (a *App) enqueueCampaignRecipients(ctx, campaign, recipients, now, fallbackStatus) error` | `campaigns.go:648-696` |
| `createRSVPReminderCampaign` — builds campaign, tx-commits, then enqueues at `:144` | `internal/handlers/rsvp_reminder_campaign.go:32` |
| Nil-contact guests silently skipped (`result.Skipped++; continue`) | `rsvp_reminder_campaign.go:81-84` |
| Recipient phone written raw | `rsvp_reminder_campaign.go:94` |
| `rsvpReminderTemplate` validates only `status == APPROVED` | `internal/handlers/rsvp_reminders.go:95` |
| `RSVPReminderPreview` counts nil-contact rows as eligible (preload without check) | `rsvp_reminders.go:168-173` |
| `SendRSVPReminders` | `rsvp_reminders.go:176` |
| `func normalizePhoneDigits(s string) string` — strips to digits | `internal/handlers/rsvp_capture.go:13` |
| RSVP capture prefixes bare 10-digit numbers with `91` | `rsvp_capture.go` (finalizeRSVPFromSession) |
| `normalizeCampaignRecipientPhone` trims whitespace + leading `+` **only** | `campaigns.go:1238-1242` |
| `const CampaignSourceRSVPReminder = "rsvp_reminder"` | `internal/models/bulk.go:9` |
| `CampaignStatus` constants | `internal/models/constants.go:122-132` |
| `BulkMessageCampaign` / `BulkMessageRecipient` structs | `internal/models/bulk.go:12-41` / `:48-65` |
| Campaign media routes | `cmd/whatomate/main.go:767-768` |
| Reminder dialog | `frontend/src/components/rsvp/RSVPReminderDialog.vue` (POST at `:171`) |

**Live evidence this is real:** campaign `6c9831a4-cf38-4198-bfa1-26cd668e34d2` (`source_type: rsvp_reminder`, 15/07/2026) reported `status: completed`, `total_recipients: 1008`, `sent_count: 0`, `failed_count: 1008`. Every recipient carried:
`failed to send template message: API error 132012: (#132012) Parameter format does not match format in the created template - Details: header component parameter should not be empty`.
The only template in the org, `rsvp_message_1`, has `HeaderType: "VIDEO"`.

---

## File Structure

| File | Responsibility | Change |
| --- | --- | --- |
| `internal/handlers/campaigns.go` | Campaign lifecycle | Modify `validateCampaignReadyForStart` (accept local path) |
| `internal/handlers/campaigns_media_validate_test.go` | Validator unit tests | Create |
| `internal/handlers/rsvp_reminder_media.go` | Staging upload for RSVP reminder media | Create |
| `internal/handlers/rsvp_reminder_media_test.go` | Staging tests | Create |
| `internal/handlers/rsvp_reminder_campaign.go` | RSVP reminder campaign build + dispatch | Modify: attach media, validate pre-enqueue, normalize/dedupe phones, record skips |
| `internal/handlers/rsvp_reminder_phone.go` | Phone normalization + dedupe for reminder recipients | Create |
| `internal/handlers/rsvp_reminder_phone_test.go` | Phone tests | Create |
| `internal/handlers/rsvp_reminders.go` | Preview + send endpoints | Modify: preview/send agreement, accept staged media |
| `cmd/whatomate/main.go` | Routes | Modify: register staging route |
| `frontend/src/components/rsvp/RSVPReminderDialog.vue` | Reminder UI | Modify: conditional media upload |

---

## Task 1: ~~Make the validator check the field the worker actually sends~~ — WRONG, REVERTED

> **RETRACTED 2026-07-17. This task was based on a misreading and must be reverted (see Task 8).**
>
> The premise — that the validator checks `HeaderMediaID`/`HeaderMediaURL` while the worker sends
> `HeaderMediaLocalPath` — is false. Verified against source:
> - `worker.go:122` sends `sendTemplateMessage(..., campaign.HeaderMediaID, campaign.HeaderMediaURL)`.
> - `worker.go:144-147` assigns `HeaderMediaLocalPath` to `message.MediaURL` only so the media
>   *"renders in the chat bubble"* — a local UI record, not the send.
> - `pkg/whatsapp/message.go:384` attaches a header only `if headerMediaID != "" || headerMediaLink != ""`.
>
> So the original validator was **correct**. Widening it to accept `HeaderMediaLocalPath` created a
> false accept: a campaign passes validation, then sends no header, and Meta rejects it with 132012 —
> the exact failure this plan exists to prevent. **Task 8 reverts this.**
>
> Historical text follows for the record; do not implement it.

The validator accepts a campaign when `HeaderMediaID` or `HeaderMediaURL` is set, but `worker.go:144-145` sends `HeaderMediaLocalPath`. A campaign with only a local path is rejected despite being sendable. `UploadCampaignMedia` writes in two phases (`campaigns.go:1665-1683`) — it clears `header_media_url`/`header_media_id` first, then sets the public URL — so a failure between those writes leaves a permanently unstartable campaign with the file on disk.

This change only ever turns a false rejection into an accept, so it cannot newly break a working campaign. It is a shared path: four call sites.

**Files:**
- Modify: `internal/handlers/campaigns.go:633-637`
- Test: `internal/handlers/campaigns_media_validate_test.go` (create)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `validateCampaignReadyForStart(campaign *models.BulkMessageCampaign) error` — unchanged signature, widened acceptance. Task 3 calls it.

- [ ] **Step 1: Write the failing test**

Create `internal/handlers/campaigns_media_validate_test.go`:

```go
package handlers

import (
	"testing"

	"github.com/nikyjain/whatomate/internal/models"
)

func TestValidateCampaignReadyForStart_AcceptsLocalPathOnly(t *testing.T) {
	// worker.go:144-145 sends HeaderMediaLocalPath, so a campaign carrying only
	// a local path is sendable and must not be rejected.
	app := &App{}
	campaign := &models.BulkMessageCampaign{
		Template:             &models.Template{HeaderType: "VIDEO"},
		HeaderMediaLocalPath: "campaigns/abc.mp4",
	}

	if err := app.validateCampaignReadyForStart(campaign); err != nil {
		t.Fatalf("expected local path to satisfy media header, got: %v", err)
	}
}

func TestValidateCampaignReadyForStart_RejectsWhenAllMediaFieldsEmpty(t *testing.T) {
	app := &App{}
	campaign := &models.BulkMessageCampaign{
		Template: &models.Template{HeaderType: "VIDEO"},
	}

	err := app.validateCampaignReadyForStart(campaign)
	if err == nil {
		t.Fatal("expected rejection when no media is present")
	}
	if got, want := err.Error(), "template requires video header media. Configure campaign media before starting"; got != want {
		t.Fatalf("error text changed:\n got: %q\nwant: %q", got, want)
	}
}

func TestValidateCampaignReadyForStart_AcceptsIDOrURL(t *testing.T) {
	app := &App{}

	byID := &models.BulkMessageCampaign{
		Template:      &models.Template{HeaderType: "IMAGE"},
		HeaderMediaID: "media-123",
	}
	if err := app.validateCampaignReadyForStart(byID); err != nil {
		t.Fatalf("HeaderMediaID must still satisfy: %v", err)
	}

	byURL := &models.BulkMessageCampaign{
		Template:       &models.Template{HeaderType: "DOCUMENT"},
		HeaderMediaURL: "https://example.test/f.pdf",
	}
	if err := app.validateCampaignReadyForStart(byURL); err != nil {
		t.Fatalf("HeaderMediaURL must still satisfy: %v", err)
	}
}

func TestValidateCampaignReadyForStart_TextHeaderNeedsNoMedia(t *testing.T) {
	app := &App{}
	campaign := &models.BulkMessageCampaign{
		Template: &models.Template{HeaderType: "TEXT"},
	}
	if err := app.validateCampaignReadyForStart(campaign); err != nil {
		t.Fatalf("TEXT header must not require media: %v", err)
	}
}

func TestValidateCampaignReadyForStart_WhitespaceOnlyLocalPathIsNotMedia(t *testing.T) {
	app := &App{}
	campaign := &models.BulkMessageCampaign{
		Template:             &models.Template{HeaderType: "VIDEO"},
		HeaderMediaLocalPath: "   ",
	}
	if err := app.validateCampaignReadyForStart(campaign); err == nil {
		t.Fatal("whitespace-only local path must not count as media")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
go test ./internal/handlers/ -run TestValidateCampaignReadyForStart -v
```

Expected: `TestValidateCampaignReadyForStart_AcceptsLocalPathOnly` **FAILS** with
`expected local path to satisfy media header, got: template requires video header media. Configure campaign media before starting`.
The other four **PASS** (they already describe current behaviour — they are regression guards).

- [ ] **Step 3: Widen the check**

In `internal/handlers/campaigns.go`, replace the `switch` at `:633-637`:

```go
	switch campaign.Template.HeaderType {
	case "IMAGE", "VIDEO", "DOCUMENT":
		if strings.TrimSpace(campaign.HeaderMediaID) == "" &&
			strings.TrimSpace(campaign.HeaderMediaURL) == "" &&
			strings.TrimSpace(campaign.HeaderMediaLocalPath) == "" {
			return fmt.Errorf("template requires %s header media. Configure campaign media before starting", strings.ToLower(campaign.Template.HeaderType))
		}
	}
```

Do not change the error string — `Step 1`'s regression test pins it, and it is user-facing.

- [ ] **Step 4: Run the tests to verify they pass**

```bash
go test ./internal/handlers/ -run TestValidateCampaignReadyForStart -v
```

Expected: all five PASS.

- [ ] **Step 5: Run the full campaign suite for regressions**

```bash
go test ./internal/handlers/ -run 'TestCampaign|TestValidateCampaign' -v
```

Expected: PASS (or SKIP where `TEST_DATABASE_URL` is unset). If anything fails, stop — the four call sites at `campaigns.go:577`, `:972`, `:1088`, `campaign_scheduler.go:61` share this function.

- [ ] **Step 6: Commit**

```bash
git add internal/handlers/campaigns.go internal/handlers/campaigns_media_validate_test.go
git commit -m "Accept HeaderMediaLocalPath as valid campaign header media

validateCampaignReadyForStart checked HeaderMediaID/HeaderMediaURL, but
worker.go:144-145 sends HeaderMediaLocalPath. A campaign carrying only a
local path was rejected despite being sendable, and UploadCampaignMedia's
two-phase write (campaigns.go:1665-1683) can leave exactly that state.

Mirrors the existing tri-field check at campaigns.go:1841. Only widens
acceptance, so it cannot newly reject a previously valid campaign."
```

---

## Task 2: Normalize and dedupe reminder recipient phones

`rsvp_reminder_campaign.go:94` writes `row.PhoneNumber` raw. `normalizeCampaignRecipientPhone` (`campaigns.go:1238-1242`) only trims whitespace and a leading `+` — it will **not** merge `9840445616` and `919840445616`. RSVP capture already has the right rule (`rsvp_capture.go`): strip to digits, prefix bare 10-digit numbers with `91`.

**This is defensive, not a diagnosed defect.** The 1,008-vs-976 gap has a duller explanation (elapsed time between the 15/07 send and the 16/07 reading). Do not claim otherwise in commit messages.

**Files:**
- Create: `internal/handlers/rsvp_reminder_phone.go`
- Test: `internal/handlers/rsvp_reminder_phone_test.go`

**Interfaces:**
- Consumes: `normalizePhoneDigits(s string) string` from `rsvp_capture.go:13`.
- Produces:
  - `func normalizeRSVPReminderPhone(phone string) string` — digits; bare 10-digit gets `91` prefix; `""` if no digits.
  - `func dedupeRSVPReminderRows[T any](rows []T, phoneOf func(T) string) (kept []T, dropped []T)` — first-wins by normalized phone; rows normalizing to `""` are **kept** (Task 4 rejects them with a reason rather than silently dropping).

  Task 4 calls both.

- [ ] **Step 1: Write the failing test**

Create `internal/handlers/rsvp_reminder_phone_test.go`:

```go
package handlers

import "testing"

func TestNormalizeRSVPReminderPhone(t *testing.T) {
	cases := []struct{ in, want string }{
		{"919840445616", "919840445616"},
		{"9840445616", "919840445616"},   // bare 10-digit gets 91, matching rsvp_capture.go
		{"+91 98404 45616", "919840445616"},
		{"+91-98404-45616", "919840445616"},
		{" 919840445616 ", "919840445616"},
		{"", ""},
		{"abc", ""},
		{"12345", "12345"},               // too short to be a bare Indian mobile: left alone
	}
	for _, c := range cases {
		if got := normalizeRSVPReminderPhone(c.in); got != c.want {
			t.Errorf("normalizeRSVPReminderPhone(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDedupeRSVPReminderRowsFirstWins(t *testing.T) {
	type row struct{ id, phone string }
	rows := []row{
		{"a", "919840445616"},
		{"b", "9840445616"},      // same person, different format
		{"c", "+91 98404 45616"}, // same person again
		{"d", "919999999999"},
	}

	kept, dropped := dedupeRSVPReminderRows(rows, func(r row) string { return r.phone })

	if len(kept) != 2 {
		t.Fatalf("expected 2 kept, got %d: %+v", len(kept), kept)
	}
	if kept[0].id != "a" || kept[1].id != "d" {
		t.Fatalf("expected first-wins a,d; got %+v", kept)
	}
	if len(dropped) != 2 {
		t.Fatalf("expected 2 dropped, got %d: %+v", len(dropped), dropped)
	}
}

func TestDedupeRSVPReminderRowsKeepsUnnormalizable(t *testing.T) {
	// Rows with no digits must survive dedupe so the caller can reject them
	// with a visible reason instead of them vanishing silently.
	type row struct{ id, phone string }
	rows := []row{{"a", ""}, {"b", "abc"}}

	kept, dropped := dedupeRSVPReminderRows(rows, func(r row) string { return r.phone })

	if len(kept) != 2 {
		t.Fatalf("unnormalizable rows must be kept, got %d: %+v", len(kept), kept)
	}
	if len(dropped) != 0 {
		t.Fatalf("expected nothing dropped, got %+v", dropped)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/handlers/ -run 'TestNormalizeRSVPReminderPhone|TestDedupeRSVPReminderRows' -v
```

Expected: FAIL to build — `undefined: normalizeRSVPReminderPhone`, `undefined: dedupeRSVPReminderRows`.

- [ ] **Step 3: Implement**

Create `internal/handlers/rsvp_reminder_phone.go`:

```go
package handlers

// normalizeRSVPReminderPhone reduces a phone to comparable digits using the same
// rule as RSVP capture (rsvp_capture.go): strip to digits, and prefix a bare
// 10-digit Indian mobile with 91 so it matches the format WhatsApp reports.
//
// campaigns.go's normalizeCampaignRecipientPhone is deliberately not reused: it
// only trims whitespace and a leading "+", so it cannot merge 9840445616 with
// 919840445616.
func normalizeRSVPReminderPhone(phone string) string {
	digits := normalizePhoneDigits(phone)
	if len(digits) == 10 {
		return "91" + digits
	}
	return digits
}

// dedupeRSVPReminderRows removes rows whose phone normalizes to one already seen,
// keeping the first occurrence. Rows that normalize to "" are kept, not dropped —
// the caller records them as skipped with a reason rather than losing them.
func dedupeRSVPReminderRows[T any](rows []T, phoneOf func(T) string) (kept []T, dropped []T) {
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		key := normalizeRSVPReminderPhone(phoneOf(row))
		if key == "" {
			kept = append(kept, row)
			continue
		}
		if _, dup := seen[key]; dup {
			dropped = append(dropped, row)
			continue
		}
		seen[key] = struct{}{}
		kept = append(kept, row)
	}
	return kept, dropped
}
```

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/handlers/ -run 'TestNormalizeRSVPReminderPhone|TestDedupeRSVPReminderRows' -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/handlers/rsvp_reminder_phone.go internal/handlers/rsvp_reminder_phone_test.go
git commit -m "Add phone normalization and dedupe for RSVP reminder recipients

Reuses the RSVP capture rule (digits, 91-prefix for bare 10-digit) rather
than normalizeCampaignRecipientPhone, which only trims whitespace and a
leading + and so cannot merge 9840445616 with 919840445616.

Defensive: no duplicate send has been observed in production data."
```

---

## Task 3: Validate before enqueue, and report an all-failed send honestly

Two independent silences, fixed together because they share the dispatch path and one test setup.

`createRSVPReminderCampaign` calls `enqueueCampaignRecipients` directly at `:144`, bypassing `validateCampaignReadyForStart` — the gate `StartCampaign` runs at `campaigns.go:577`. And a campaign where every recipient failed still reports `status: completed`, which is how 1,008 failures went unnoticed.

**Files:**
- Modify: `internal/handlers/rsvp_reminder_campaign.go`
- Test: `internal/handlers/rsvp_reminder_campaign_test.go` (create)

**Interfaces:**
- Consumes: `validateCampaignReadyForStart` (Task 1) — called directly. Do **not** add an RSVP-specific wrapper around it; it would be pure delegation and its test would duplicate Task 1's.
- Produces: `func rsvpReminderCampaignOutcome(sent, failed, total int) string`, and `rsvpReminderCampaignResult` gains `Skipped []rsvpReminderSkip`. Task 4 populates it; Task 6 renders it.
  ```go
  type rsvpReminderSkip struct {
      ResponseID uuid.UUID `json:"response_id"`
      Name       string    `json:"name"`
      Phone      string    `json:"phone"`
      Reason     string    `json:"reason"`
  }
  ```

- [ ] **Step 1: Write the failing test**

Create `internal/handlers/rsvp_reminder_campaign_test.go`. This asserts the ordering contract: validation must run **before** anything is queued.

```go
package handlers

import (
	"testing"
)

func TestRSVPReminderCampaignOutcomeAllFailed(t *testing.T) {
	cases := []struct {
		name       string
		sent, fail int
		total      int
		want       string
	}{
		{"all failed", 0, 1008, 1008, "failed"},
		{"partial", 900, 108, 1008, "completed_with_errors"},
		{"clean", 1008, 0, 1008, "completed"},
		{"empty", 0, 0, 0, "completed"},
	}
	for _, c := range cases {
		if got := rsvpReminderCampaignOutcome(c.sent, c.fail, c.total); got != c.want {
			t.Errorf("%s: rsvpReminderCampaignOutcome(%d,%d,%d) = %q, want %q",
				c.name, c.sent, c.fail, c.total, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/handlers/ -run TestRSVPReminder -v
```

Expected: FAIL to build — `undefined: rsvpReminderCampaignOutcome`.

- [ ] **Step 3: Implement**

Add to `internal/handlers/rsvp_reminder_campaign.go` (top level, after the imports):

```go
// rsvpReminderCampaignOutcome classifies a finished reminder campaign. A run where
// every recipient failed must not present as a clean success — that is how 1008
// consecutive failures went unnoticed on 15/07/2026.
func rsvpReminderCampaignOutcome(sent, failed, total int) string {
	switch {
	case total > 0 && sent == 0 && failed >= total:
		return "failed"
	case failed > 0:
		return "completed_with_errors"
	default:
		return "completed"
	}
}
```

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/handlers/ -run TestRSVPReminder -v
```

Expected: all PASS.

- [ ] **Step 5: Wire validation into the dispatch path**

In `internal/handlers/rsvp_reminder_campaign.go`, immediately **before** the `enqueueCampaignRecipients` call at `:144` — and **after** the campaign has its media fields set (Task 4 sets them) — insert:

```go
	// The RSVP path calls enqueueCampaignRecipients directly and so never passed
	// through StartCampaign's gate (campaigns.go:577). Without this, a media-header
	// template fails once per recipient with Meta error 132012 — 1008 times on
	// 15/07/2026, while the campaign reported "completed".
	if err := a.validateCampaignReadyForStart(campaign); err != nil {
		return nil, err
	}
```

Match the function's existing return signature. If it returns `(*rsvpReminderCampaignResult, error)`, the above is correct as written; if it returns a bare `error`, drop the `nil,`. Read the signature at `:32` before editing — do not guess.

- [ ] **Step 6: Verify the whole package still builds and passes**

```bash
go build ./... && go test ./internal/handlers/ -run 'TestRSVP|TestCampaign|TestValidateCampaign' -v
```

Expected: build OK; tests PASS or SKIP.

- [ ] **Step 7: Commit**

```bash
git add internal/handlers/rsvp_reminder_campaign.go internal/handlers/rsvp_reminder_campaign_test.go
git commit -m "Validate RSVP reminders before enqueue and classify all-failed runs

The RSVP path called enqueueCampaignRecipients directly, bypassing the
validation StartCampaign runs at campaigns.go:577. On 15/07/2026 that let
1008 recipients each fail with Meta error 132012 while the campaign
reported status completed.

Validate up front, and classify a run where every recipient failed as
failed rather than completed."
```

---

## Task 4: Stop silently dropping guests; make preview and send agree

`rsvp_reminder_campaign.go:81-84` skips `row.Contact == nil` with `result.Skipped++; continue` — no delivery row, no log, no reason. Meanwhile `RSVPReminderPreview` (`rsvp_reminders.go:168-173`) preloads Contact but never checks it, so it counts those same rows as eligible. The admin is promised N and gets M < N, with no way to learn who vanished.

**Files:**
- Modify: `internal/handlers/rsvp_reminder_campaign.go:81-84`
- Modify: `internal/handlers/rsvp_reminders.go:168-173`
- Test: `internal/handlers/rsvp_reminder_skips_test.go` (create)

**Interfaces:**
- Consumes: `rsvpReminderSkip` (Task 3), `dedupeRSVPReminderRows` + `normalizeRSVPReminderPhone` (Task 2).
- Produces: `func rsvpReminderSkipReason(hasContact bool, phone string) string` — `""` when sendable, else a human reason. Used by both preview and send so they cannot drift.

- [ ] **Step 1: Write the failing test**

Create `internal/handlers/rsvp_reminder_skips_test.go`:

```go
package handlers

import "testing"

func TestRSVPReminderSkipReason(t *testing.T) {
	cases := []struct {
		name       string
		hasContact bool
		phone      string
		want       string
	}{
		{"sendable", true, "919840445616", ""},
		{"no contact", false, "919840445616", "no contact record"},
		{"no phone", true, "", "no usable phone number"},
		{"unusable phone", true, "abc", "no usable phone number"},
		{"no contact wins over phone", false, "", "no contact record"},
	}
	for _, c := range cases {
		if got := rsvpReminderSkipReason(c.hasContact, c.phone); got != c.want {
			t.Errorf("%s: rsvpReminderSkipReason(%v, %q) = %q, want %q",
				c.name, c.hasContact, c.phone, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/handlers/ -run TestRSVPReminderSkipReason -v
```

Expected: FAIL to build — `undefined: rsvpReminderSkipReason`.

- [ ] **Step 3: Implement**

Add to `internal/handlers/rsvp_reminder_campaign.go`:

```go
// rsvpReminderSkipReason returns "" when a guest can be sent to, otherwise a
// human-readable reason. Preview and send both call this so their counts cannot
// drift: previously send dropped nil-contact rows (rsvp_reminder_campaign.go:81)
// while preview counted them as eligible (rsvp_reminders.go:168).
func rsvpReminderSkipReason(hasContact bool, phone string) string {
	if !hasContact {
		return "no contact record"
	}
	if normalizeRSVPReminderPhone(phone) == "" {
		return "no usable phone number"
	}
	return ""
}
```

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/handlers/ -run TestRSVPReminderSkipReason -v
```

Expected: PASS.

- [ ] **Step 5: Replace the silent skip in the send path**

In `internal/handlers/rsvp_reminder_campaign.go`, replace the guard at `:81-84`. Read the loop's actual variable names first — `row`, `result`, and the recipient slice — and adapt:

```go
		if reason := rsvpReminderSkipReason(row.Contact != nil, row.PhoneNumber); reason != "" {
			result.Skipped = append(result.Skipped, rsvpReminderSkip{
				ResponseID: row.ID,
				Name:       row.RecipientName(),
				Phone:      row.PhoneNumber,
				Reason:     reason,
			})
			a.Log.Warn("RSVP reminder skipped guest",
				"rsvp_response_id", row.ID, "reason", reason, "event_id", event.ID)
			continue
		}
```

If `row` has no `RecipientName()` helper, use the contact's name field directly, guarding for nil — check the struct at `rsvp_guests.go:126-131` (`rsvpGuestRosterRow`) before writing this. Keep the existing `result.Skipped++` counter working if other code reads it; if `Skipped` becomes a slice, replace reads with `len(result.Skipped)`.

- [ ] **Step 6: Apply dedupe in the same loop**

Before the recipient loop in `createRSVPReminderCampaign`, after rows are loaded (around `:54`):

```go
	rows, duplicates := dedupeRSVPReminderRows(rows, func(r rsvpGuestRosterRow) string { return r.PhoneNumber })
	for _, dup := range duplicates {
		result.Skipped = append(result.Skipped, rsvpReminderSkip{
			ResponseID: dup.ID,
			Phone:      dup.PhoneNumber,
			Reason:     "duplicate phone number",
		})
	}
```

Use the real element type of `rows` — read it at `:54`, do not assume `rsvpGuestRosterRow`.

- [ ] **Step 7: Make preview agree with send**

In `internal/handlers/rsvp_reminders.go:168-173`, `RSVPReminderPreview` currently counts every preloaded row as eligible. Apply the same predicate:

```go
	eligible := 0
	skipped := make([]rsvpReminderSkip, 0)
	for _, row := range rows {
		if reason := rsvpReminderSkipReason(row.Contact != nil, row.PhoneNumber); reason != "" {
			skipped = append(skipped, rsvpReminderSkip{
				ResponseID: row.ID,
				Phone:      row.PhoneNumber,
				Reason:     reason,
			})
			continue
		}
		eligible++
	}
```

Return `skipped` alongside `eligible` in the preview envelope so the dialog can list who will not be messaged and why.

- [ ] **Step 8: Verify build and tests**

```bash
go build ./... && go test ./internal/handlers/ -run TestRSVP -v
```

Expected: build OK; PASS or SKIP.

- [ ] **Step 9: Commit**

```bash
git add internal/handlers/rsvp_reminder_campaign.go internal/handlers/rsvp_reminders.go internal/handlers/rsvp_reminder_skips_test.go
git commit -m "Surface skipped RSVP reminder guests instead of dropping them

Send dropped nil-contact guests with no delivery row, no log and no
reason, while preview counted the same rows as eligible - so the admin
was promised more than was sent and could not tell who vanished.

Preview and send now share rsvpReminderSkipReason and both report the
skipped set. Duplicate phones are reported rather than silently merged."
```

---

## Task 5: Accept header media for a reminder send

The RSVP reminder creates and enqueues its campaign in one call, but `UploadCampaignMedia` (`campaigns.go:1591`) needs an existing campaign ID. So media must be staged first and attached during creation.

**Files:**
- Create: `internal/handlers/rsvp_reminder_media.go`
- Modify: `cmd/whatomate/main.go` (route)
- Modify: `internal/handlers/rsvp_reminder_campaign.go` (attach staged media)
- Test: `internal/handlers/rsvp_reminder_media_test.go`

**Interfaces:**
- Consumes: `saveCampaignMedia` (`campaigns.go:1701`), `detectCampaignMediaMimeType` (`campaigns.go:1651`), and the validation call wired in by Task 3.
- Produces:
  - Route `POST /api/rsvp/{id}/reminders/media` → `(a *App) UploadRSVPReminderMedia(r *fastglue.Request) error`, returning `{"staging_id","filename","mime_type"}`.
  - `func rsvpReminderStagingKey(stagingID string) string` — the staging path.
  - Send request accepts `staging_id`.

**Correction (2026-07-17), verified against the source — read before implementing.**
`saveCampaignMedia` (`campaigns.go:1703-1730`) does **not** accept an arbitrary path:

```go
subdir := "campaigns"
if err := a.ensureMediaDir(subdir); err != nil { ... }   // creates ONLY "campaigns"
filename := campaignID + ext
filePath := filepath.Join(a.getMediaStoragePath(), subdir, filename)
...
relativePath := filepath.Join(subdir, filename)          // already "campaigns/<id><ext>"
return relativePath, nil
```

Consequences the original draft of this task got wrong:
- Passing `"staging/<id>"` as `campaignID` would try to write `campaigns/staging/<id><ext>`, but
  `ensureMediaDir` (`internal/handlers/media.go:27`) only ever creates `campaigns` — the write
  would fail with "no such file or directory".
- `saveCampaignMedia` **already returns** the `campaigns/`-prefixed path **with** the extension. So
  reconstructing `"campaigns/" + key` would both double-prefix and drop the extension.

**Therefore: no staging subdirectory.** Use a `staging-<uuid>` filename inside the existing
`campaigns` directory, and store the value `saveCampaignMedia` **returns** — never a reconstructed
path.

- [ ] **Step 1: Write the failing test**

Create `internal/handlers/rsvp_reminder_media_test.go`:

```go
package handlers

import (
	"strings"
	"testing"
)

func TestRSVPReminderStagingKeyIsScoped(t *testing.T) {
	// The value is passed to saveCampaignMedia as its campaignID, which writes to
	// <media>/campaigns/<value><ext>. It must stay a flat filename: ensureMediaDir
	// only creates "campaigns", so any subdirectory would fail to write.
	key := rsvpReminderStagingKey("abc123")
	if !strings.HasPrefix(key, "staging-") {
		t.Fatalf("staging key must be a flat staging- filename, got %q", key)
	}
	if !strings.Contains(key, "abc123") {
		t.Fatalf("staging key must contain the staging id, got %q", key)
	}
	if strings.ContainsAny(key, `/\`) {
		t.Fatalf("staging key must contain no path separator, got %q", key)
	}
}

func TestRSVPReminderStagingKeyRejectsTraversal(t *testing.T) {
	// staging_id arrives from the client and is used to build a filesystem path.
	for _, bad := range []string{"../secrets", "a/b", "..", "a\\b", "", "a.b"} {
		if got := rsvpReminderStagingKey(bad); got != "" {
			t.Errorf("rsvpReminderStagingKey(%q) = %q, want \"\" (rejected)", bad, got)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/handlers/ -run TestRSVPReminderStagingKey -v
```

Expected: FAIL to build — `undefined: rsvpReminderStagingKey`.

- [ ] **Step 3: Implement the staging key**

Create `internal/handlers/rsvp_reminder_media.go`:

```go
package handlers

import (
	"regexp"
)

// stagingIDPattern constrains a staging id to characters that cannot escape the
// media directory. The id reaches us from the client and is used to build a
// filesystem path, so anything outside this set is refused rather than sanitized.
var stagingIDPattern = regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`)

// rsvpReminderStagingKey returns the pseudo campaign id under which a staged
// reminder media file is saved, or "" if the id is not safe to use in a path.
//
// It is deliberately a flat "staging-<id>" filename, not a subdirectory:
// saveCampaignMedia hardcodes subdir "campaigns" and ensureMediaDir creates only
// that, so "staging/<id>" would fail to write.
func rsvpReminderStagingKey(stagingID string) string {
	if !stagingIDPattern.MatchString(stagingID) {
		return ""
	}
	return "staging-" + stagingID
}
```

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/handlers/ -run TestRSVPReminderStagingKey -v
```

Expected: PASS.

- [ ] **Step 5: Implement the upload handler**

Add to `internal/handlers/rsvp_reminder_media.go`. Copy the multipart handling, the 16MB cap and the MIME sniffing from `UploadCampaignMedia` (`campaigns.go:1591-1698`) — read it and mirror it rather than inventing a second convention. The handler must:

1. Resolve org via `a.getOrgID(r)`; 401 on failure.
2. Resolve the event via `parsePathUUID(r, "id", "RSVP event")` and confirm it belongs to the org; 404 otherwise.
3. Read the file with `io.LimitReader(file, maxMediaSize+1)`; reject over 16MB with `"File too large. Maximum size is 16MB"`.
4. `mimeType := detectCampaignMediaMimeType(fileHeader.Filename, fileHeader.Header.Get("Content-Type"), data)`; reject unsupported with `"Unsupported file type: "+mimeType`.
5. `stagingID := uuid.New().String()` — note `uuid.New().String()` contains only hex and `-`, so it satisfies `stagingIDPattern`. Then `saveCampaignMedia(rsvpReminderStagingKey(stagingID), data, mimeType)`.
6. Return `r.SendEnvelope(map[string]interface{}{"staging_id": stagingID, "filename": fileHeader.Filename, "mime_type": mimeType})`.

Do **not** return or trust a client-supplied path. The send re-derives the storage location from
`staging_id` via `rsvpReminderStagingKey`.

- [ ] **Step 6: Register the route**

In `cmd/whatomate/main.go`, beside the existing RSVP reminder routes:

```go
	g.POST("/api/rsvp/{id}/reminders/media", app.UploadRSVPReminderMedia)
```

Apply the same auth/permission middleware as the neighbouring `/api/rsvp/{id}/reminders` routes — copy their exact wrapper, do not invent one.

- [ ] **Step 7: Attach staged media during campaign creation**

In `createRSVPReminderCampaign`, after the campaign struct is built (`:64-75`) and **before** the validation added in Task 3, set the media fields from the staged file when a `staging_id` was supplied:

```go
	if stagingID != "" {
		key := rsvpReminderStagingKey(stagingID)
		if key == "" {
			return result, fmt.Errorf("invalid media reference")
		}
		// The staged file already exists on disk under this pseudo campaign id.
		// Re-derive its stored path the same way saveCampaignMedia built it, rather
		// than reconstructing a path by hand: saveCampaignMedia returns
		// "campaigns/<id><ext>", so a hand-built "campaigns/"+key would both
		// double-prefix and drop the extension.
		campaign.HeaderMediaLocalPath = filepath.Join("campaigns", key+getExtensionFromMimeType(stagingMimeType))
		campaign.HeaderMediaFilename = stagingFilename
		campaign.HeaderMediaMimeType = stagingMimeType
		campaign.HeaderMediaID = ""
		campaign.HeaderMediaURL = ""
	}
```

`getExtensionFromMimeType` is the same helper `saveCampaignMedia` uses (`campaigns.go:1705`) — read it
and confirm the extension it yields matches. **Verify the path you store actually resolves to the file
on disk**; the worker reads `HeaderMediaLocalPath` (`worker.go:144-145`) and a wrong path here
reproduces the very failure this plan fixes, just with a different error.

Note the return is `result, err` — `createRSVPReminderCampaign` returns `rsvpReminderCampaignResult`
by **value**, not a pointer (Task 3 confirmed this against the source).

Thread `stagingID`, `stagingFilename` and `stagingMimeType` from the send request through `SendRSVPReminders` (`rsvp_reminders.go:176`) into `createRSVPReminderCampaign`. Add them to the request struct the dialog POSTs to at `RSVPReminderDialog.vue:171`.

Do **not** call `populateCampaignHeaderMediaFromURL` here: it swallows `saveCampaignMedia` failures (`campaigns.go:437-439`), clobbering `HeaderMediaLocalPath` to `""` and returning `nil` — the exact silent-partial-success this plan exists to remove.

- [ ] **Step 8: Verify build and full suite**

```bash
go build ./... && go test ./internal/handlers/ -v
```

Expected: build OK; PASS or SKIP.

- [ ] **Step 9: Commit**

```bash
git add internal/handlers/rsvp_reminder_media.go internal/handlers/rsvp_reminder_media_test.go internal/handlers/rsvp_reminder_campaign.go internal/handlers/rsvp_reminders.go cmd/whatomate/main.go
git commit -m "Accept header media for RSVP reminder sends

The reminder campaign is created and enqueued in one call, but
UploadCampaignMedia needs an existing campaign id, so media is staged
first and attached during creation.

populateCampaignHeaderMediaFromURL is deliberately not reused: it
swallows saveCampaignMedia failures and returns nil with an empty local
path."
```

---

## Task 6: Reminder dialog — upload media, show skips, report failure

**Files:**
- Modify: `frontend/src/components/rsvp/RSVPReminderDialog.vue`

**Interfaces:**
- Consumes: `POST /api/rsvp/{id}/reminders/media` → `{staging_id, filename, mime_type}` (Task 5); preview `skipped[]` (Task 4).
- Produces: none.

- [ ] **Step 1: Add the conditional upload control**

The dialog already has a template picker (`Reminder template ID`) and a `Template variables` mapper. When the selected template's `header_type` is `IMAGE`, `VIDEO` or `DOCUMENT`, show a file input labelled with the required type. When it is `TEXT` or empty, show nothing.

The template list is already fetched for the picker; read `header_type` from the same objects — do not add a second fetch.

- [ ] **Step 2: Block the send until media is attached**

Disable **Remind all not started** and **Remind selected** while the chosen template needs media and none is staged. Show the reason inline, e.g.:

> `rsvp_message_1 needs a video. Attach one to send.`

This is the user-facing half of Task 3's server-side gate. Both must exist: the server is the guarantee, the UI is the courtesy.

- [ ] **Step 3: Upload on file select**

POST the file to `/api/rsvp/{id}/reminders/media` as multipart, keep the returned `staging_id`, `filename` and `mime_type` in component state, and include them in the existing send POST body (`:171`).

- [ ] **Step 4: Show who will be skipped**

Render the preview's `skipped[]` above the send buttons, e.g. `3 guests will not be messaged` expanding to name, phone and reason. Previously these guests vanished with no trace.

- [ ] **Step 5: Report the outcome honestly**

On send, if the response reports `sent == 0 && failed > 0`, show an error, not a success toast. Include the first recipient's `error_message` — that string is what identified this bug (`(#132012) ... header component parameter should not be empty`) and it is the most useful thing on the screen.

- [ ] **Step 6: Typecheck and lint**

```bash
cd frontend && npm run typecheck && npm run lint
```

Expected: both clean.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/components/rsvp/RSVPReminderDialog.vue
git commit -m "Add media upload, skip list and honest failures to reminder dialog

Blocks the send when the chosen template needs header media and none is
attached, lists guests who will not be messaged and why, and reports a
run where every recipient failed as a failure rather than a success."
```

---

## Task 7: End-to-end verification against the real failure

No new code. This proves the 15/07 failure cannot recur.

- [ ] **Step 1: Full test suite**

```bash
go build ./... && make test
```

Expected: PASS (DB-backed tests SKIP without `TEST_DATABASE_URL`; set it to run them for real, which is strongly preferred before deploying this).

- [ ] **Step 2: Frontend build**

```bash
cd frontend && npm run typecheck && npm run build
```

Expected: clean.

- [ ] **Step 3: Reproduce the original failure, and confirm it is now refused**

Against a dev instance with an RSVP event whose reminder template has a VIDEO header:

1. Open the Reminders dialog. Select the media-header template. Attach nothing.
2. **Expect:** send buttons disabled, with a message naming the missing video.
3. Force it server-side, bypassing the UI:
   ```bash
   curl -X POST "$BASE/api/rsvp/$EVENT_ID/reminders/send" \
     -H 'Content-Type: application/json' -H "Cookie: $SESSION" \
     -d '{"all_not_started":true,"template_id":"'$TEMPLATE_ID'","template_params":{"1":"{{member_name}}"}}'
   ```
   **Expect:** a 4xx naming the missing header media. **Expect:** no campaign row created, and **zero** messages queued. Before this plan, this call created a campaign and burned 1,008 sends.
4. Attach a video, send to a **single** test recipient. **Expect:** `sent: 1, failed: 0`, and the message arrives with its video.

- [ ] **Step 4: Confirm the shared path is unharmed**

Create an ordinary campaign through the Campaigns UI with the same media-header template, attach media, start it, send to one test recipient. **Expect:** unchanged behaviour. Task 1 touched a function this path shares.

- [ ] **Step 5: Commit any fixes**

If steps 3-4 surface problems, fix and commit before this plan is considered done. Do not mark it complete on a green unit suite alone — the unit tests passed for the code that failed 1,008 times.

---

## Task 8: Revert the false validator widening, and actually attach media Meta can fetch

**Added 2026-07-17 after review found Tasks 1 and 5 were built on a misreading.** This task is what
makes the send genuinely work. Until it lands, the branch is *worse* than `main`: validation has a
hole in it and the RSVP path clears the only two fields that matter.

**The verified truth:**

| Field | What it is actually for |
| --- | --- |
| `HeaderMediaID` | Sent to Meta (`worker.go:122` → `sendTemplateMessage`) |
| `HeaderMediaURL` | Sent to Meta (same call). A publicly fetchable URL. |
| `HeaderMediaLocalPath` | **Local UI only** — `worker.go:144-147` assigns it to `message.MediaURL` so the media *"renders in the chat bubble"*. Never sent. |

`pkg/whatsapp/message.go:384` attaches a header component only `if headerMediaID != "" || headerMediaLink != ""`.

**Why the manual 13/07 campaign worked and the RSVP 15/07 one didn't:** `UploadCampaignMedia` ends
with a second write setting `header_media_url` to a public URL (`campaigns.go:1681-1687`, via
`buildPublicCampaignMediaURL`). The RSVP path set no media field at all.

**Files:**
- Modify: `internal/handlers/campaigns.go` (revert Task 1's widening)
- Modify: `internal/handlers/campaigns_media_validate_test.go` (revert Task 1's test)
- Modify: `internal/handlers/rsvp_reminder_campaign.go` (set a fetchable URL)
- Modify: `internal/handlers/rsvp_reminders.go` (thread the request through, if needed)

- [ ] **Step 1: Revert the validator widening**

Restore the original three-line check in `validateCampaignReadyForStart` — `HeaderMediaID` or
`HeaderMediaURL` only. Delete `TestValidateCampaignReadyForStart_AcceptsLocalPathOnly` and add its
inverse, which pins the corrected understanding:

```go
func TestValidateCampaignReadyForStart_RejectsLocalPathOnly(t *testing.T) {
	// HeaderMediaLocalPath is for local chat rendering (worker.go:144-147); the send
	// uses HeaderMediaID/HeaderMediaURL (worker.go:122). A campaign carrying only a
	// local path sends no header component and Meta rejects it with 132012, so it
	// must NOT pass validation.
	app := &App{}
	campaign := &models.BulkMessageCampaign{
		Template:             &models.Template{HeaderType: "VIDEO"},
		HeaderMediaLocalPath: "campaigns/abc.mp4",
	}
	if err := app.validateCampaignReadyForStart(campaign); err == nil {
		t.Fatal("a local path alone must not satisfy a media header - nothing would be sent")
	}
}
```

Keep the other four Task 1 tests — they were always correct.

- [ ] **Step 2: Set a URL Meta can actually fetch**

In `createRSVPReminderCampaign`, the staged file must end up reachable. Mirror `UploadCampaignMedia`
(`campaigns.go:1591-1698`) — read it and follow it exactly:

1. After the campaign row exists (it is created in the tx, so `campaign.ID` is known), save the
   staged bytes under the campaign's own id via `saveCampaignMedia(campaign.ID.String(), data, mimeType)`,
   exactly as the normal path does. This also removes the reliance on a client-echoed MIME type for
   path building — derive `mimeType` from the staged upload server-side.
2. Build the public URL with `buildPublicCampaignMediaURL` and assign it to `campaign.HeaderMediaURL`,
   persisting it — this is the field that reaches Meta.
3. Keep `HeaderMediaLocalPath` set as well, so the chat bubble renders. It is additive, not a substitute.
4. **Then** validate, **then** enqueue. Ordering is unchanged from Task 3.

`buildPublicCampaignMediaURL(r *fastglue.Request, campaign *models.BulkMessageCampaign) string`
needs the request (for `requestPublicBaseURL`). `createRSVPReminderCampaign` is also called by
`rsvp_scheduler.go`, which has no request. Decide and state your choice in the report:
- thread an optional base URL / request through, and for the scheduler derive the base URL from
  config; **or**
- have the handler set the URL after creation and before enqueue.
Do not leave the scheduler path silently URL-less — that is a 1,008-failure repeat with a different
trigger.

- [ ] **Step 3: Prove it end-to-end, not just in unit tests**

The unit tests passed for the code that failed 1,008 times. Before claiming done, verify on a dev
instance that a reminder using a VIDEO-header template arrives **with its video** at one test number.
If you cannot run a real send, say so plainly in the report rather than implying it was verified.

- [ ] **Step 4: Verify and commit**

```bash
go build ./... && go vet ./internal/handlers/... && go test ./internal/handlers/ -v
```

```bash
git add -A internal/handlers/
git commit -m "Send header media Meta can fetch; revert false validator widening

worker.go:122 sends HeaderMediaID/HeaderMediaURL. HeaderMediaLocalPath is
assigned to message.MediaURL at worker.go:144-147 purely so media renders
in the local chat bubble - it is never sent. An earlier change misread
that line and widened validateCampaignReadyForStart to accept a local
path, which let a campaign pass validation and then send no header at
all: Meta error 132012, the exact failure this branch exists to fix.

Revert the widening and populate HeaderMediaURL via
buildPublicCampaignMediaURL, mirroring UploadCampaignMedia - which is why
the manual 13/07 campaign sent 1,273 while the 15/07 RSVP reminder failed
1,008/1,008."
```

---

## Deferred (explicitly NOT in this plan)

Logged in the spec, deliberately excluded — do not scope-creep into them:

- Orphaned campaign when post-commit enqueue fails (`rsvp_reminder_campaign.go:144-146`): recipients persist `pending`, deliveries `queued`, no reaper.
- Scheduler swallows the all-skipped case (`rsvp_scheduler.go:69-72`).
- Header-**text** placeholders never collected (`rsvp_reminders.go:33` scans body + buttons only).
- Flat param namespace collision between body and URL-button names.
- `saveCampaignMedia` overwriting a campaign's media history in place (`campaigns/<id><ext>`).
- Whether `rsvp_message_1` should remain both invite and reminder template — a content decision for the user.

---

## Self-Review

- **Spec coverage.** Section 1's six required changes map to tasks: attach media → Task 5+6; pre-flight validation → Task 1+3; honest reporting → Task 3+6; normalize/dedupe → Task 2+4; stop dropping contactless guests → Task 4; scheduled reminders inherit → Task 3 (validation sits in the shared `createRSVPReminderCampaign`, which `rsvp_scheduler.go` calls). ✅
- **Independently deployable.** No task references headcount contributors or follow-up campaigns. ✅
- **Type consistency.** `rsvpReminderSkip` defined once (Task 3), used in Tasks 4 and 6. `normalizeRSVPReminderPhone` and `dedupeRSVPReminderRows` defined in Task 2, used in Task 4. `validateRSVPReminderCampaignSendable` defined in Task 3, used in Task 5. `rsvpReminderStagingKey` defined in Task 5, used in Task 5 only. ✅
- **Known soft spots** — flagged inline for the implementer rather than guessed at: the exact return signature of `createRSVPReminderCampaign` (Task 3 Step 5), the element type of `rows` (Task 4 Step 6), whether `saveCampaignMedia` prefixes `campaigns/` (Task 5 Step 3), and the name field on the roster row (Task 4 Step 5). Each step says to read before editing.
