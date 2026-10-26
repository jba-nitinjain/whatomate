import { describe, expect, it } from 'vitest'
import {
  formatDate,
  formatDateDDMMYYYY,
  formatDateTime,
  formatDateTimeIST,
  formatDayMonth,
  formatDayMonthYear
} from './utils'

// Local-time constructors keep these assertions independent of the runner's timezone.
const june9 = new Date(2026, 5, 9, 14, 5, 30)
const jan2 = new Date(2026, 0, 2, 7, 4)

describe('formatDate', () => {
  it('formats Date objects as dd/mm/yyyy with zero padding', () => {
    expect(formatDate(june9)).toBe('09/06/2026')
    expect(formatDate(jan2)).toBe('02/01/2026')
  })

  it('formats ISO strings as dd/mm/yyyy', () => {
    expect(formatDate('2026-12-31T10:00:00')).toBe('31/12/2026')
  })

  it('returns an empty string for empty or invalid input', () => {
    expect(formatDate('')).toBe('')
    expect(formatDate(null)).toBe('')
    expect(formatDate(undefined)).toBe('')
    expect(formatDate('not-a-date')).toBe('')
  })
})

describe('formatDateTime', () => {
  it('formats as dd/mm/yyyy HH:mm using a 24-hour clock', () => {
    expect(formatDateTime(june9)).toBe('09/06/2026 14:05')
    expect(formatDateTime(jan2)).toBe('02/01/2026 07:04')
  })

  it('accepts ISO strings and rejects invalid input', () => {
    expect(formatDateTime('2026-03-04T23:59:00')).toBe('04/03/2026 23:59')
    expect(formatDateTime('nope')).toBe('')
    expect(formatDateTime(null)).toBe('')
  })
})

describe('calendar helpers', () => {
  it('formatDayMonthYear pads day and month (1-based month)', () => {
    expect(formatDayMonthYear(2026, 6, 9)).toBe('09/06/2026')
    expect(formatDayMonthYear(2026, 11, 25)).toBe('25/11/2026')
  })

  it('formatDayMonth gives dd/mm for chart axes', () => {
    expect(formatDayMonth(june9)).toBe('09/06')
    expect(formatDayMonth('bad')).toBe('')
  })

  it('formatDateDDMMYYYY stays an alias of formatDate', () => {
    expect(formatDateDDMMYYYY(june9)).toBe(formatDate(june9))
  })
})

describe('formatDateTimeIST', () => {
  it('formats a UTC instant as dd/mm/yyyy HH:mm in Asia/Kolkata', () => {
    expect(formatDateTimeIST('2026-06-09T08:35:00Z')).toBe('09/06/2026 14:05')
    expect(formatDateTimeIST('2026-12-31T20:00:00Z')).toBe('01/01/2027 01:30')
  })
})
