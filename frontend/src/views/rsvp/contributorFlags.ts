export interface RSVPContributor {
  label: string
  answer_key: string
  mode: string
  people: number
  responses: number
  needs_review: number
  unparseable: number
}

// A family is never silently lost: this is the count shown as a warning on a
// contributor card so unparseable/ambiguous answers surface on the dashboard
// itself rather than only in the export.
export function contributorFlagCount(c: RSVPContributor): number {
  return (c.needs_review || 0) + (c.unparseable || 0)
}
