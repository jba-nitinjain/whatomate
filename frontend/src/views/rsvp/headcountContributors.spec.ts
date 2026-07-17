import { describe, it, expect } from 'vitest'
import {
  parseMatchValues,
  matchValuesToText,
  contributorsToRows,
  legacyHeadcountContributorRows,
  contributorRowsToPayload,
} from './headcountContributors'

describe('parseMatchValues', () => {
  it('splits and trims a comma-separated list', () => {
    expect(parseMatchValues('yes, attending')).toEqual(['yes', 'attending'])
  })

  it('drops empty entries from stray commas', () => {
    expect(parseMatchValues('yes,, attending,')).toEqual(['yes', 'attending'])
  })

  it('returns an empty array for blank input', () => {
    expect(parseMatchValues('')).toEqual([])
    expect(parseMatchValues('   ')).toEqual([])
  })
})

describe('matchValuesToText', () => {
  it('joins values with a comma and space', () => {
    expect(matchValuesToText(['yes', 'attending'])).toBe('yes, attending')
  })

  it('tolerates missing values', () => {
    expect(matchValuesToText(undefined)).toBe('')
    expect(matchValuesToText(null)).toBe('')
    expect(matchValuesToText([])).toBe('')
  })
})

describe('contributorsToRows', () => {
  it('converts saved contributors into editable rows with match_values as text', () => {
    const rows = contributorsToRows([
      { label: 'Spouse', answer_key: 'spouse_attendance', mode: 'boolean', match_values: ['yes', 'attending'] },
    ])
    expect(rows).toHaveLength(1)
    expect(rows[0].label).toBe('Spouse')
    expect(rows[0].answer_key).toBe('spouse_attendance')
    expect(rows[0].mode).toBe('boolean')
    expect(rows[0].match_values_text).toBe('yes, attending')
  })

  it('falls back to boolean mode for an unrecognised mode value', () => {
    const rows = contributorsToRows([{ label: 'Odd', answer_key: 'x', mode: 'nonsense' }])
    expect(rows[0].mode).toBe('boolean')
  })

  it('returns an empty list for no contributors', () => {
    expect(contributorsToRows(undefined)).toEqual([])
    expect(contributorsToRows(null)).toEqual([])
    expect(contributorsToRows([])).toEqual([])
  })

  it('assigns each row a distinct client key', () => {
    const rows = contributorsToRows([
      { label: 'A', answer_key: 'a', mode: 'boolean', match_values: ['yes'] },
      { label: 'B', answer_key: 'b', mode: 'boolean', match_values: ['yes'] },
    ])
    expect(rows[0]._key).not.toBe(rows[1]._key)
  })
})

describe('legacyHeadcountContributorRows', () => {
  it('prefills member attendance and spouse attendance, matching the server default', () => {
    const rows = legacyHeadcountContributorRows()
    expect(rows).toHaveLength(2)
    expect(rows[0].mode).toBe('attendance')
    expect(rows[0].answer_key).toBe('')
    expect(rows[0].match_values_text).toBe('yes')
    expect(rows[1].mode).toBe('boolean')
    expect(rows[1].answer_key).toBe('spouse_attendance')
    expect(rows[1].match_values_text).toBe('yes, attending')
  })

  it('round-trips through contributorRowsToPayload back into the shape legacyHeadcountContributors produces', () => {
    const payload = contributorRowsToPayload(legacyHeadcountContributorRows())
    expect(payload).toEqual([
      { label: 'Member attendance', answer_key: '', mode: 'attendance', match_values: ['yes'] },
      { label: 'Spouse attendance', answer_key: 'spouse_attendance', mode: 'boolean', match_values: ['yes', 'attending'] },
    ])
  })
})

describe('contributorRowsToPayload', () => {
  it('trims label and answer_key', () => {
    const payload = contributorRowsToPayload([
      { _key: 1, label: '  Children  ', answer_key: '  children_count  ', mode: 'numeric', match_values_text: '' },
    ])
    expect(payload[0].label).toBe('Children')
    expect(payload[0].answer_key).toBe('children_count')
  })

  it('omits match_values for numeric rows', () => {
    const payload = contributorRowsToPayload([
      { _key: 1, label: 'Children', answer_key: 'children_count', mode: 'numeric', match_values_text: 'yes' },
    ])
    expect(payload[0].match_values).toBeUndefined()
  })

  it('clears answer_key for attendance rows even if one is left over from another mode', () => {
    const payload = contributorRowsToPayload([
      { _key: 1, label: 'Member', answer_key: 'attendance', mode: 'attendance', match_values_text: 'yes' },
    ])
    expect(payload[0].answer_key).toBe('')
    expect(payload[0].match_values).toEqual(['yes'])
  })
})
