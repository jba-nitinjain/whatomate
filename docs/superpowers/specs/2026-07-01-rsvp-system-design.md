# RSVP System — Design Spec

**Date:** 2026-07-01
**Project:** Whatomate (WhatsApp Business Platform — Go backend, Vue3 + shadcn-vue frontend, multi-tenant)
**Status:** Approved for planning

---

## 1. Summary

Add an **RSVP** capability as a standard, first-class feature available to every organization — delivered as a "chat flow like RSVP." An **RSVP Event** ties together a chatbot flow (the dynamic questions), guest delivery (bulk campaign + keyword/link), structured collected answers, a live results dashboard, Excel export, auto-reminders to non-responders, and an RSVP close/cutoff date.

Approach: a **thin layer over existing subsystems** (chatbot flows, campaigns, keyword rules, scheduler ticker, campaign XLSX export). ~70% of the plumbing already exists; RSVP adds the event/response data model, a flow "save answer" step, entry-point wiring, reminders, cutoff, and the results UI.

### Not building (rejected alternatives)
- **Standalone RSVP subsystem** with its own question builder + delivery — duplicates the flow builder and campaigns. Rejected.
- **Meta-native WhatsApp Flow forms** for RSVP — each form needs Meta approval, weak branching, harder dynamic tally. Noted as a possible future add-on, out of scope here.

---

## 2. Goals

- RSVP available to all organizations (standard feature), org-scoped and isolated.
- Questions and follow-up questions are **fully dynamic and per-event + per-org** — authored in the existing chatbot flow builder (branching via button/list/condition nodes).
- **Multiple RSVP events run concurrently** within an org without response cross-contamination.
- Two entry points: bulk campaign to a guest list, and keyword / wa.me link for open invites.
- Live results dashboard + guest list, Excel export, auto-reminders to non-responders, and RSVP close/cutoff date.

### Non-goals
- Ticketing, payments, seating charts, calendar sync — out of scope.
- Public web RSVP pages — WhatsApp-only entry for this iteration.

---

## 3. Architecture

### 3.1 Data model — `internal/models/rsvp.go` (GORM, registered in AutoMigrate in `internal/database/postgres.go`)

**`RSVPEvent`**
- `ID`, `OrganizationID` (scoping)
- `Name`, `Description`
- `EventDate` (the event itself), `RSVPCloseAt` (cutoff for accepting responses)
- `FlowID` — linked chatbot flow that asks the questions
- `Keyword` — entry keyword for keyword/link path (nullable if campaign-only)
- `Status` — `draft` / `active` / `closed`
- Reminder settings: `ReminderEnabled` (bool), `ReminderAt` (timestamp) or `ReminderOffset` (before cutoff), `ReminderTemplateID`
- `AccountID` (which WhatsApp account/number sends), timestamps

**`RSVPResponse`**
- `ID`, `EventID`, `OrganizationID`
- `ContactID`
- `Status` — `yes` / `no` / `maybe` / `pending`
- `Answers` — JSONB map of dynamic `field name → value` (headcount, meal choice, notes, anything the flow captures)
- `RespondedAt` (nullable while pending), timestamps
- Unique constraint on (`EventID`, `ContactID`) → upsert on repeat answers.

### 3.2 Handlers — `internal/handlers/rsvp.go`
- Event CRUD (list/create/get/update/delete), all filtered by `organization_id` (mirror `campaigns.go`).
- List responses for an event; live **tally** endpoint (counts by status + headcount sum + per-field breakdown).
- **XLSX export** of guest list + answers — reuse `campaign_report_workbook.go` pattern.
- Enforce validation rules (see §5).

### 3.3 Flow capture — new "Save RSVP answer" flow action
- Extend the chatbot flow builder/processor (`internal/handlers/chatbot_processor.go`, node types `start/text/button/list/condition/action`) with a new **action**: `save_rsvp_answer`.
- Maps a reply (button/list/text) → a named field written into `RSVPResponse.Answers`.
- One field flagged as **attendance** sets `RSVPResponse.Status` (yes/no/maybe).
- Because answers are keyed by field name in JSONB, editing a flow mid-event tolerates missing/new fields.

### 3.4 Entry points
- **Bulk campaign** — reuse Campaigns. An invite template with a button starts the linked flow. When a campaign is tied to an RSVP event, its recipients seed as `RSVPResponse` rows with `status = pending` and known `event_id`.
- **Keyword / wa.me link** — reuse keyword rules. Guest messages the event keyword (or taps a `wa.me` link with the keyword prefilled) → starts the same flow. If the sender is not a known contact, auto-create the contact, then the response.

### 3.5 Reminders
- New `ProcessDueRSVPReminders` hooked onto the **existing scheduler ticker** (alongside `ProcessDueCampaigns` in `campaign_scheduler.go`).
- On each tick: for active events whose `ReminderAt` is due and not yet past `RSVPCloseAt`, re-send the invite (via `ReminderTemplateID`, respecting the WhatsApp 24-hour window and contact opt-out) to all responses still `pending`.

### 3.6 Cutoff / close
- Flow entry checks `RSVPCloseAt`. Replies after cutoff are rejected with a friendly "sorry, RSVP is closed" message and do not mutate responses.
- Event `Status` flips to `closed` after cutoff; results dashboard locks (read-only + export still available).

### 3.7 Frontend — `frontend/src/views/rsvp/` (Vue3 + shadcn-vue)
- `RSVPEventsView.vue` — list of events (name, dates, status, response counts).
- `RSVPEventBuilderView.vue` — create/edit event: name, description, event date, RSVP close date, attach or create a flow, entry keyword, sending account, reminder toggle + time + template.
- `RSVPResultsView.vue` — live count cards (Yes / No / Maybe / Pending + headcount sum), guest table (contact + each answer), Export button.
- Sidebar nav entry **"RSVP"**.
- All dates displayed **dd/mm/yyyy** (store ISO internally, format at display).

---

## 4. Concurrency: multiple events at once

Multiple RSVP events may be `active` simultaneously within one org. Which event a guest is answering is resolved per entry path:

- **Campaign path** — recipient rows are seeded with `event_id` upfront → event is always known.
- **Keyword / link path** — resolved by keyword. **Rule: an event's `Keyword` must be unique among all `active` events within the same organization.** Validation blocks creating/activating an event whose keyword collides with another active event in that org. (Keywords may be reused once the earlier event is `closed`.)
- **Mid-flow switch** — if a guest is partway through Event A's flow and enters Event B (new invite/keyword), the latest entry wins: Event B starts fresh, Event A's partial response remains `pending`. No cross-contamination.

Every `RSVPResponse` is tagged with `event_id` + `organization_id`, so tallies, dashboards, and exports never mix events or orgs.

---

## 5. Validation & edge cases

- **Unique keyword per active event per org** — enforced on create/activate (see §4). Clear error on collision.
- **Double reply** — upsert on (`event_id`, `contact_id`); latest answer wins, `responded_at` updated.
- **Unknown sender (keyword path)** — auto-create contact, then create response.
- **Reply after cutoff** — rejected + friendly closed message; no mutation.
- **Flow edited mid-event** — answers keyed by field name in JSONB; missing/new fields tolerated.
- **Reminders** — respect WhatsApp 24-hour session window (send via template) and contact opt-out; never remind non-`pending` guests; never remind past cutoff.
- **Permissions** — new `rsvp` resource added to roles/permissions (read/create/update/delete), consistent with existing granular permission model.

---

## 6. Data flow (end to end)

1. Create RSVP Event → set dates, close date, reminder.
2. Build/attach the chat flow (dynamic questions + branching follow-ups) with `save_rsvp_answer` actions.
3. Send invites: bulk campaign to a guest list (seeds `pending`) and/or publish keyword / wa.me link.
4. Guest answers in WhatsApp chat → flow writes answers into `RSVPResponse.Answers`, attendance sets `Status`.
5. Results dashboard aggregates live (counts, headcount, per-field breakdown).
6. Reminder ticker re-invites still-`pending` guests before cutoff.
7. Cutoff closes the event; export guest list + answers to XLSX.

---

## 7. Reuse map

| Need | Reused from |
|------|-------------|
| Dynamic questions + branching follow-ups | Chatbot flow builder + `chatbot_processor.go` |
| Bulk invite delivery + recipient seeding | Campaigns (`campaigns.go`) |
| Open/keyword entry | Keyword rules |
| Auto-reminders | Scheduler ticker (`campaign_scheduler.go`) |
| Excel export | `campaign_report_workbook.go` |
| Org scoping, auth, roles/permissions | Existing multi-tenant + permission model |

---

## 8. Testing

Go unit tests mirroring existing `*_test.go` conventions:
- Event CRUD (org-scoped isolation).
- Response upsert on repeat answers.
- Tally aggregation (counts, headcount sum, per-field breakdown).
- Cutoff logic (reply after close rejected).
- Reminder selection (only `pending`, before cutoff, opt-out respected).
- **Unique-keyword-per-active-event** validation (collision blocked; reuse allowed after close).
- Flow-capture: simulate button/list/text answers → assert `RSVPResponse` rows and `Answers` contents.
