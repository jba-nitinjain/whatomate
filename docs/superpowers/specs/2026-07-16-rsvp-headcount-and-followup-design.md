# RSVP Headcount Contributors & Follow-Up Campaigns — Design

**Date:** 2026-07-16
**Status:** Approved (design), pending implementation plan
**Related:** `2026-07-01-rsvp-system-design.md`

---

## Context

The live RSVP event (`892adc2c-29ec-4b87-b9b6-84274523d0c2`, "RSVP", event date **19/07/2026**) has
1,276 guests: 271 member-yes, 28 member-no, 207 spouse-yes, 977 pending.

Two needs drove this design:

1. Collect a **children headcount** from members and guests, without hard-coding children anywhere.
2. Reach the **271 guests who already responded** — currently impossible — to top up their record
   with the children count.

Investigation of the live application surfaced a third, urgent issue: **RSVP reminders have never
successfully sent a single message.**

### Findings from the live application

- The results grid **auto-generates columns** from answer keys (`rsvp.go:515-525`, `jsonb_each_text`
  over `*_title` keys). A new question yields a new column with no code. Per-guest children
  visibility is therefore free.
- The dashboard has **no numeric aggregation** and no grand total. It renders Member/Spouse
  attendance cards only.
- The results grid renders **"Spouse Attendance" twice** — once for the raw answer key (`yes`) and
  once for its `_title` companion (`Attending`). Cosmetic bug.
- The spouse tally is **hard-coded** to the magic key `spouse_attendance` (`rsvp_tally.go:52`),
  while everything else is configurable. Renaming that question in the flow builder silently zeroes
  the spouse card.
- The account contains exactly **one WhatsApp template**: `rsvp_message_1` — APPROVED, MARKETING,
  **VIDEO header**, body `Dear *Shri/Smt* *{{1}}*, You are invited to register for the *Golden …*`,
  buttons `Attending` / `Not Attending` (QUICK_REPLY). The Reminders dialog's template picker works
  correctly; it offers one option because one template exists.
- Header media is stored **per campaign**, not per template. The successful 13/07 campaign
  (1,273 sent / 919 delivered / 764 read) had `WhatsApp_Video_2026-07-10.mp4` attached to it.
- **The reminder campaign of 15/07 09:30 UTC** (`6c9831a4-cf38-4198-bfa1-26cd668e34d2`,
  `source_type: rsvp_reminder`) reported `status: completed` with
  **`total_recipients: 1008`, `sent_count: 0`, `failed_count: 1008`**. Every recipient failed with:

  > `API error 132012: (#132012) Parameter format does not match format in the created template
  > - Details: header component parameter should not be empty`

---

## Decisions (confirmed with the user)

| Decision | Choice |
| --- | --- |
| Children data granularity | **Headcount only** — no names, no age bands |
| Question shape | **Yes/No, then count**; count widget (buttons vs typed) is the flow author's choice per event |
| Reporting | **Total + per-guest column** in the results grid, filterable and exportable |
| Grand total | **Yes** — show combined `Total attending` (members + spouses + children) |
| Architecture | **Generic headcount contributors** — no children-specific code |
| Reaching prior responders | **Follow-up top-up** — do not reset, do not chase manually |
| Send path | **Campaigns** for both reminders and follow-ups, with template picker + dynamic fields |
| Header media | **Uploaded per send**, not stored on the template |

---

## Section 1 — Fix the reminder send (prerequisite, urgent)

### Root cause

The RSVP reminder path builds a campaign and dispatches it directly via
`enqueueCampaignRecipients` (`rsvp_reminder_campaign.go:144`), **bypassing
`validateCampaignReadyForStart`** — the gate the normal campaign path runs at `campaigns.go:577`
(StartCampaign) and `campaigns.go:1088` (ImportRecipients auto-queue). That gate rejects a template
with `HeaderType` IMAGE/VIDEO/DOCUMENT and no `HeaderMediaID`/`HeaderMediaURL`.

`rsvpReminderTemplate` (`rsvp_reminders.go:95`) validates only `status == APPROVED`. The campaign
struct (`rsvp_reminder_campaign.go:64-75`) never sets `HeaderMediaURL` / `HeaderMediaID` /
`HeaderMediaLocalPath`. `BuildTemplateComponents` (`pkg/whatsapp/message.go:384`) then drops the
header component entirely, and Meta rejects with 132012.

`rsvp_message_1` has a VIDEO header. Therefore **100% of RSVP reminders fail, and always have.**

### Required changes

1. **Attach header media per send.** The Reminders dialog gains a media upload control that appears
   only when the selected template declares a media header. The uploaded media populates the
   campaign's header media fields, mirroring the normal campaign path
   (`populateCampaignHeaderMediaFromURL`, `campaigns.go:207-214`).
2. **Pre-flight validation.** Call `validateCampaignReadyForStart` before enqueueing. Refuse the
   send and surface the reason in the UI rather than failing N times downstream.
3. **Honest reporting.** A campaign where `sent_count == 0 && failed_count == total_recipients` must
   not present as a clean `completed`. Surface `0 sent, N failed` in the RSVP UI and the campaigns
   list.
4. **Normalize and dedupe phone numbers.** Route `row.PhoneNumber` through
   `normalizeCampaignRecipientPhone` and dedupe, matching `ImportRecipients`
   (`campaigns.go:1109-1152`). Currently `rsvp_reminder_campaign.go:94` uses the raw value, so
   `9840445616` and `919840445616` are two recipients. This is the likely cause of the 1,008 vs ~976
   discrepancy. It also breaks `RetryFailed`'s inactive-contact filter, which keys on the normalized
   phone (`campaigns.go:834`).
5. **Stop silently dropping contactless guests.** `rsvp_reminder_campaign.go:81-84` skips
   `row.Contact == nil` with no delivery row, no log, no reason — while
   `RSVPReminderPreview` (`rsvp_reminders.go:168-173`) counts those same rows as eligible. Preview
   and send must agree, and skipped guests must be listed with a reason.
6. **Scheduled reminders** use the same path and must inherit all of the above
   (`rsvp_scheduler.go`).

### Deferred (logged, not in scope)

- **Orphaned campaign on post-commit enqueue failure** (`rsvp_reminder_campaign.go:144-146`):
  recipients persist as `pending`, deliveries as `queued`, with no reaper. Recovery via
  StartCampaign is possible but unexposed.
- **Scheduler swallows the all-skipped case** (`rsvp_scheduler.go:69-72`): `Campaign == nil` marks
  the schedule `completed` whether everyone legitimately responded or every guest was dropped by the
  nil-Contact guard.
- **Header-text placeholders never collected** (`rsvp_reminders.go:33` scans `BodyContent` +
  `Buttons` only). Pre-existing; consistent with the chat path.
- **Flat param namespace collision** between body and URL-button param names
  (`rsvp_reminders.go:33`). Pre-existing.

### Operational note

`rsvp_message_1` is configured as **both** the invite and the reminder template. Once fixed, the 976
pending members receive the original invite video as their reminder. A dedicated reminder template
is a content decision for the user, not a code change.

---

## Section 2 — Asking for the children count (no code)

Authored per event in the existing flow builder — no code, no schema:

> **"Are you bringing children?"** → Yes / No
> *(if Yes)* **"How many children?"** → quick-reply buttons or free text, author's choice

The results grid grows a **Children** column automatically.

**In scope:** fix the duplicated **"Spouse Attendance"** column (raw key and `_title` key both
render). Collapse to one column per logical question before a third question compounds it.

---

## Section 3 — Headcount contributors (the build)

Replace the hard-coded spouse tally with a per-event, ordered list of **headcount contributors**
stored on the RSVP event:

| Field | Meaning |
| --- | --- |
| `label` | Card title on the dashboard, e.g. "Children" |
| `answer_key` | Which answer holds the value |
| `mode` | `boolean` (counts 1 when the answer matches) or `numeric` (counts the value given) |
| `match_values` | For `boolean` mode — which answers mean yes |

Seed configuration reproduces today's behaviour exactly:

| Label | Key | Mode | Counts |
| --- | --- | --- | --- |
| Member attendance | *(attendance field)* | boolean | 1 per Yes |
| Spouse attendance | `spouse_attendance` | boolean | 1 per Yes |
| Children | *(author's key)* | numeric | the value given |

Each contributor renders a dashboard card and feeds a new **Total attending** figure
(members + spouses + children). On current data: 271 + 207 = 478 before children.

**This removes the `spouse_attendance` magic string** (`rsvp_tally.go:52`) — the one place a flow
author can silently zero a dashboard card by renaming a question.

### Numeric parsing

Lenient, since the count widget may be free text: extract the first integer (`"3"`, `"3 kids"` → 3);
map number words zero–ten (`"three"` → 3); treat "no"/"none"/empty as 0. **Unparseable values count
as 0 and are flagged in the grid** — never silently dropped. Negative values clamp to 0. Values
above a sane ceiling (say 20) are flagged for review rather than rejected.

---

## Section 4 — Follow-up campaigns (generic)

### The gap

Every existing send targets non-responders:

| Action | Audience |
| --- | --- |
| Send invitations | First contact |
| Reminders | `loadNotStartedRSVPGuests` — never started only |
| Re-prompt pending | Pending / mid-flow |

Nothing reaches responders, and the keyword path actively rejects them via `DuplicateMessage`
("Thank you! Your RSVP has been already recorded.").

### Design

A **Follow-up** action on the Results page, built from three pickers — no children-specific logic:

1. **Audience filter** — `not_started` | `responded_yes` | `responded_no` | **`missing_answer(key)`**
2. **Template** — any template from the list, dynamic field mapping, header media upload (Section 1)
3. **Flow** — which flow runs when the recipient replies

Children = `missing_answer(children_count)` → new template → children flow.

**Why `missing_answer` is the primary filter:** it is self-cleaning. Respondents drop out of the
audience as answers arrive, so re-sending chases only the genuinely missing. No list maintenance, no
double-messaging, and it generalises to any question added in future.

### Answer merge semantics

- The follow-up flow asks **only** its own questions.
- Answers **merge into the existing `rsvp_responses.answers` JSONB**. Attendance, spouse, and all
  prior answers are preserved.
- `attendance` is **not** recomputed from a follow-up.
- The duplicate guard is **bypassed for follow-up sessions only**; the main RSVP entry path keeps
  its existing protection.
- Follow-up sessions are tagged so recovery/replay (`rsvp_recover.go`) does not confuse them with a
  primary RSVP.

### External dependency (blocking, not code)

A **new Meta template** must be created and approved, carrying a tap button such as
"Tell us about children". The sole existing template's buttons are `Attending` / `Not Attending` —
unsuitable. Meta approval is the long pole and must start immediately.

---

## Sequencing

All four sections are built together as one body of work (user decision, 2026-07-16).

**Constraint:** Section 4 is gated on Meta approving a new template — an external dependency with an
uncontrollable timeline, against an event on **19/07/2026**. Bundling delivery of all four sections
behind that approval would block the reminder fix, leaving 976 pending members unreminded.

**Therefore: build together, but keep Section 1 independently deployable.** Section 1 must not
acquire a code dependency on Sections 2–4 and must be releasable on its own the moment it is tested,
regardless of the state of the rest. Sections 2–4 follow when ready.

Practical ordering within the work:

1. **Section 1 — reminder fix.** No external dependency. Deployable standalone.
2. **Section 2 — children questions + duplicate-column fix.** Configuration + a small grid fix.
   The new Meta template is submitted for approval at the start of the work, not the end.
3. **Section 3 — headcount contributors.** Delivers the total; removes the `spouse_attendance`
   magic string.
4. **Section 4 — follow-up campaigns.** Code can be complete and tested ahead of Meta approval;
   only the live send waits on it.

---

## Testing

- **Section 1:** a media-header template with no media is **rejected pre-flight**, not sent.
  A media-header template with media uploaded sends successfully. Phone variants
  (`9840445616` / `919840445616`) collapse to one recipient. Contactless guests appear in the
  skipped list with a reason, and preview count equals send count. A fully-failed campaign does not
  report as clean success.
- **Section 3:** contributors reproduce current tallies exactly on live data (271 member-yes,
  207 spouse-yes). Numeric parsing covers `"3"`, `"3 kids"`, `"three"`, `""`, `"none"`, `"-1"`,
  `"999"`, `"abc"`. Renaming a question no longer silently zeroes a card. Zero contributors
  configured degrades gracefully.
- **Section 4:** `missing_answer` shrinks as answers arrive. A follow-up answer merges without
  clobbering attendance or spouse. A follow-up does not trip the duplicate guard; a genuine
  duplicate RSVP still does.

## Out of scope

Age bands; named children; per-child records; children counting toward the duplicate guard;
children affecting the existing Member/Spouse cards; reworking `Send invitations`; the deferred
items listed in Section 1.
