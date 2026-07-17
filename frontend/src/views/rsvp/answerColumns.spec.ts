import { describe, it, expect } from 'vitest'
import { visibleAnswerKeys } from './answerColumns'

describe('visibleAnswerKeys', () => {
  it('drops the raw key when its _title companion exists', () => {
    // The chatbot writes both spouse_attendance ("yes") and
    // spouse_attendance_title ("Attending"); both became columns, both labelled
    // "Spouse Attendance", with different values.
    const rows = [{ answers: {
      attendance: 'yes',
      attendance_title: 'Attending',
      spouse_attendance: 'yes',
      spouse_attendance_title: 'Attending',
      spouse_mobile: '919840026019',
    } }]

    expect(visibleAnswerKeys(rows)).toEqual([
      'attendance_title',
      'spouse_attendance_title',
      'spouse_mobile',
    ])
  })

  it('keeps a raw key that has no _title companion', () => {
    const rows = [{ answers: { children_count: '2' } }]
    expect(visibleAnswerKeys(rows)).toEqual(['children_count'])
  })

  it('excludes internal underscore-prefixed keys', () => {
    const rows = [{ answers: { _rsvp_event_id: 'abc', children_count: '2' } }]
    expect(visibleAnswerKeys(rows)).toEqual(['children_count'])
  })

  it('unions keys across rows in first-seen order', () => {
    const rows = [
      { answers: { attendance_title: 'Attending' } },
      { answers: { children_count: '1' } },
      { answers: { attendance_title: 'Attending', spouse_mobile: '91' } },
    ]
    expect(visibleAnswerKeys(rows)).toEqual([
      'attendance_title',
      'children_count',
      'spouse_mobile',
    ])
  })

  it('tolerates missing or empty answers', () => {
    expect(visibleAnswerKeys([])).toEqual([])
    expect(visibleAnswerKeys([{}])).toEqual([])
    expect(visibleAnswerKeys([{ answers: {} }])).toEqual([])
  })
})
