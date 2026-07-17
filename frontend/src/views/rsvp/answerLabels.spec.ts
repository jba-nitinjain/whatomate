import { describe, it, expect } from 'vitest'
import { resolveAnswerColumnLabel, resolveContributorCardLabel } from './answerLabels'
import type { RSVPContributor } from './contributorFlags'

// A translator that makes an untranslated regression obvious: real i18n keys
// map to a distinct, non-English-looking string, so a test asserting on the
// English literal (e.g. "Spouse attendance") would fail loudly if the
// production code ever fell back to the contributor's own English label
// instead of calling t().
const t = (key: string): string => {
  const table: Record<string, string> = {
    'rsvp.memberAttendance': 'Sadasya Upasthiti',
    'rsvp.spouseAttendance': 'Jeevansathi Upasthiti',
  }
  return table[key] ?? key
}

function contributor(overrides: Partial<RSVPContributor> = {}): RSVPContributor {
  return {
    label: 'Spouse attendance',
    answer_key: 'spouse_attendance',
    mode: 'boolean',
    people: 0,
    responses: 0,
    needs_review: 0,
    unparseable: 0,
    ...overrides,
  }
}

describe('resolveAnswerColumnLabel', () => {
  it('translates the member attendance column instead of using a contributor label', () => {
    const contributors = [contributor({ label: 'Member attendance', answer_key: '', mode: 'attendance' })]
    expect(resolveAnswerColumnLabel('attendance', 'attendance', contributors, t)).toBe('Sadasya Upasthiti')
  })

  it('translates the conventional spouse_attendance column instead of using the contributor label', () => {
    // This is the exact regression a reviewer flagged: the spouse column
    // label resolved from the contributor's English literal "Spouse
    // attendance" instead of t('rsvp.spouseAttendance'). Without the fix
    // (contributor label checked before the conventional-key check), this
    // assertion fails with "Spouse attendance" instead of the translated
    // string.
    const contributors = [contributor()]
    expect(resolveAnswerColumnLabel('spouse_attendance', 'attendance', contributors, t)).toBe('Jeevansathi Upasthiti')
  })

  it('still uses the contributor own label for a user-authored row with no translation', () => {
    const contributors = [contributor({ label: 'Children', answer_key: 'children_count', mode: 'numeric' })]
    expect(resolveAnswerColumnLabel('children_count', 'attendance', contributors, t)).toBe('Children')
  })

  it('falls back to a prettified key when there is no contributor and no built-in match', () => {
    expect(resolveAnswerColumnLabel('spouse_mobile', 'attendance', [], t)).toBe('Spouse Mobile')
  })

  it('honours the _title-stripped base for the built-in columns', () => {
    const contributors = [contributor()]
    expect(resolveAnswerColumnLabel('spouse_attendance_title'.slice(0, -'_title'.length), 'attendance', contributors, t)).toBe('Jeevansathi Upasthiti')
  })
})

describe('resolveContributorCardLabel', () => {
  it('translates the attendance-mode (member) card', () => {
    const c = contributor({ label: 'Member attendance', answer_key: '', mode: 'attendance' })
    expect(resolveContributorCardLabel(c, t)).toBe('Sadasya Upasthiti')
  })

  it('translates the conventional spouse_attendance card instead of the English literal', () => {
    // Guards the "every contributor card" half of the same regression: the
    // headcount contributor summary card also read c.label directly.
    const c = contributor()
    expect(resolveContributorCardLabel(c, t)).toBe('Jeevansathi Upasthiti')
  })

  it('keeps a renamed spouse question on its own label, not the conventional translation', () => {
    // A genuinely renamed spouse question is user data (Task 9's territory),
    // not the conventional key, so it must NOT be silently translated.
    const c = contributor({ label: 'Partner attending', answer_key: 'partner_coming' })
    expect(resolveContributorCardLabel(c, t)).toBe('Partner attending')
  })

  it('keeps a user-authored row (Children) on its own label', () => {
    const c = contributor({ label: 'Children', answer_key: 'children_count', mode: 'numeric' })
    expect(resolveContributorCardLabel(c, t)).toBe('Children')
  })
})
