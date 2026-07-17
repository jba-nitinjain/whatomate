import type { RSVPContributor } from './contributorFlags'

/**
 * The historical hardcoded spouse question key (formerly rsvp_tally.go:52 on
 * the backend). It is also the answer key the event builder pre-fills for the
 * Spouse row, so it is what a contributor is keyed as unless an admin
 * deliberately retypes it.
 */
export const SPOUSE_ATTENDANCE_KEY = 'spouse_attendance'

/**
 * Resolves the display label for an answers-grid column keyed `base` (already
 * stripped of a `_title` suffix).
 *
 * The event's own attendance field and the conventional spouse_attendance key
 * always resolve through the supplied translator `t`, never through a
 * contributor's own `label` - even though a contributor's own label is
 * preferred for every other row. This is what keeps a non-English dashboard
 * from regressing to English on the two built-in columns: legacy contributors
 * carry the English literals "Member attendance" / "Spouse attendance" as
 * their `label`, and preferring that literal over `t('rsvp.memberAttendance')`
 * / `t('rsvp.spouseAttendance')` is exactly the regression this guards.
 *
 * A contributor's own label is used only as the fallback for a genuinely
 * user-authored row (e.g. "Children") that has no translation of its own -
 * that label IS user data and is correctly left untranslated.
 */
export function resolveAnswerColumnLabel(
  base: string,
  attendanceField: string,
  contributors: RSVPContributor[],
  t: (key: string) => string,
): string {
  if (base === attendanceField) return t('rsvp.memberAttendance')
  if (base === SPOUSE_ATTENDANCE_KEY) return t('rsvp.spouseAttendance')
  const contributor = contributors.find(c => c.answer_key === base)
  if (contributor?.label) return contributor.label
  return base.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase())
}

/**
 * Same precedence as resolveAnswerColumnLabel, for the per-contributor
 * summary cards, which read a contributor directly instead of going through
 * an answer key. Attendance-mode always means the member contributor (there
 * can be at most one, enforced by the backend's validateRSVPHeadcountContributors).
 */
export function resolveContributorCardLabel(
  c: RSVPContributor,
  t: (key: string) => string,
): string {
  if (c.mode === 'attendance') return t('rsvp.memberAttendance')
  if (c.answer_key === SPOUSE_ATTENDANCE_KEY) return t('rsvp.spouseAttendance')
  return c.label
}
