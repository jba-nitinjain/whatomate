// Editable headcount contributor row for the RSVP event settings form. Mirrors
// models.RSVPHeadcountContributor (internal/models/rsvp.go) except match_values
// is edited as a comma-separated string here and split into an array only when
// building the save payload - typing "yes, attending" into one input is a lot
// friendlier than editing a JSON array.
export interface HeadcountContributorRow {
  // Client-only stable identity for v-for :key across add/remove/reorder. Never
  // sent to the server.
  _key: number
  label: string
  answer_key: string
  mode: 'boolean' | 'numeric' | 'attendance'
  match_values_text: string
}

// The shape the API accepts/returns (models.RSVPHeadcountContributor as JSON).
export interface HeadcountContributorPayload {
  label: string
  answer_key: string
  mode: string
  match_values?: string[]
}

let rowKeySeq = 0
export function nextContributorRowKey(): number {
  rowKeySeq += 1
  return rowKeySeq
}

/** Splits a "yes, attending" style field into a trimmed, non-empty value list. */
export function parseMatchValues(text: string): string[] {
  return text
    .split(',')
    .map(v => v.trim())
    .filter(v => v.length > 0)
}

/** Joins a match_values array back into the comma-separated text an input edits. */
export function matchValuesToText(values: string[] | undefined | null): string {
  return (values || []).join(', ')
}

function toRow(c: HeadcountContributorPayload): HeadcountContributorRow {
  const mode = c.mode === 'numeric' || c.mode === 'attendance' ? c.mode : 'boolean'
  return {
    _key: nextContributorRowKey(),
    label: c.label || '',
    answer_key: c.answer_key || '',
    mode,
    match_values_text: matchValuesToText(c.match_values),
  }
}

/** Converts the API's saved contributors into editable rows, one row each. */
export function contributorsToRows(contributors: HeadcountContributorPayload[] | undefined | null): HeadcountContributorRow[] {
  return (contributors || []).map(toRow)
}

// Reproduces legacyHeadcountContributors (internal/handlers/rsvp_headcount.go):
// the member-attendance + spouse-attendance pair every event ran on before this
// editor existed. Used to prefill an event that has nothing configured yet, so
// saving is a no-op rather than a behaviour change - the tally handler already
// falls back to this same pair when headcount_contributors is empty.
export function legacyHeadcountContributorRows(): HeadcountContributorRow[] {
  return [
    {
      _key: nextContributorRowKey(),
      label: 'Member attendance',
      answer_key: '',
      mode: 'attendance',
      match_values_text: 'yes',
    },
    {
      _key: nextContributorRowKey(),
      label: 'Spouse attendance',
      answer_key: 'spouse_attendance',
      mode: 'boolean',
      match_values_text: 'yes, attending',
    },
  ]
}

/**
 * Builds the save payload from the editable rows. Question key is cleared for
 * attendance-mode rows (the UI disables that field, but a stale value left over
 * from switching modes must not be sent - the server ignores it for attendance
 * mode, but sending it back invites confusion and, if it happens to equal the
 * event's own attendance_field, a spurious double-count rejection).
 */
export function contributorRowsToPayload(rows: HeadcountContributorRow[]): HeadcountContributorPayload[] {
  return rows.map(row => {
    const payload: HeadcountContributorPayload = {
      label: row.label.trim(),
      answer_key: row.mode === 'attendance' ? '' : row.answer_key.trim(),
      mode: row.mode,
    }
    if (row.mode === 'boolean' || row.mode === 'attendance') {
      payload.match_values = parseMatchValues(row.match_values_text)
    }
    return payload
  })
}
