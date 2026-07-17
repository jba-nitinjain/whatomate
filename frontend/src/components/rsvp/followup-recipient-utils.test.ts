import { describe, expect, it } from 'vitest'
import {
  filterFollowUpRecipients,
  followUpRecipientLabel,
  followUpRecipientPageCount,
  isFollowUpRecipient,
  paginateFollowUpRecipients,
  type FollowUpRecipient,
} from './followup-recipient-utils'

const GUEST_ONE: FollowUpRecipient = { id: 'r-1', phone_number: '919876543210', contact: { profile_name: 'Asha Rao' } }
const GUEST_TWO: FollowUpRecipient = { id: 'r-2', phone_number: '919876543211', contact: { profile_name: 'Ben Cho' } }
const GUEST_NO_CONTACT: FollowUpRecipient = { id: 'r-3', phone_number: '919876543212' }

describe('followUpRecipientLabel', () => {
  it('prefers the contact profile name and falls back to the phone number', () => {
    expect(followUpRecipientLabel(GUEST_ONE)).toBe('Asha Rao')
    expect(followUpRecipientLabel(GUEST_NO_CONTACT)).toBe('919876543212')
  })
})

describe('isFollowUpRecipient', () => {
  it('accepts a well-formed recipient row and rejects malformed ones', () => {
    expect(isFollowUpRecipient(GUEST_ONE)).toBe(true)
    expect(isFollowUpRecipient(GUEST_NO_CONTACT)).toBe(true)
    expect(isFollowUpRecipient({ id: 'r-4' })).toBe(false)
    expect(isFollowUpRecipient({ phone_number: '919876543213' })).toBe(false)
    expect(isFollowUpRecipient(null)).toBe(false)
    expect(isFollowUpRecipient('r-5')).toBe(false)
    expect(isFollowUpRecipient(['r-6'])).toBe(false)
  })
})

describe('filterFollowUpRecipients', () => {
  const recipients = [GUEST_ONE, GUEST_TWO, GUEST_NO_CONTACT]

  it('returns every recipient for an empty or whitespace-only search', () => {
    expect(filterFollowUpRecipients(recipients, '')).toEqual(recipients)
    expect(filterFollowUpRecipients(recipients, '   ')).toEqual(recipients)
  })

  it('matches by name case-insensitively', () => {
    expect(filterFollowUpRecipients(recipients, 'asha')).toEqual([GUEST_ONE])
    expect(filterFollowUpRecipients(recipients, 'RAO')).toEqual([GUEST_ONE])
  })

  it('matches by phone number, including a guest with no contact record', () => {
    expect(filterFollowUpRecipients(recipients, '43212')).toEqual([GUEST_NO_CONTACT])
  })

  it('returns nothing when no recipient matches', () => {
    expect(filterFollowUpRecipients(recipients, 'nobody')).toEqual([])
  })
})

describe('paginateFollowUpRecipients', () => {
  const recipients = Array.from({ length: 25 }, (_, i) => ({ id: `r-${i}`, phone_number: `9${i}` }))

  it('slices the requested page at the given limit', () => {
    expect(paginateFollowUpRecipients(recipients, 1, 10)).toHaveLength(10)
    expect(paginateFollowUpRecipients(recipients, 1, 10)[0].id).toBe('r-0')
    expect(paginateFollowUpRecipients(recipients, 3, 10)).toHaveLength(5)
    expect(paginateFollowUpRecipients(recipients, 3, 10)[0].id).toBe('r-20')
  })

  it('treats an out-of-range or non-positive page as page 1 rather than throwing or returning garbage', () => {
    expect(paginateFollowUpRecipients(recipients, 0, 10)).toEqual(paginateFollowUpRecipients(recipients, 1, 10))
    expect(paginateFollowUpRecipients(recipients, -3, 10)).toEqual(paginateFollowUpRecipients(recipients, 1, 10))
  })

  it('returns an empty page past the end of the list', () => {
    expect(paginateFollowUpRecipients(recipients, 10, 10)).toEqual([])
  })
})

describe('followUpRecipientPageCount', () => {
  it('always reports at least one page, even for an empty list', () => {
    expect(followUpRecipientPageCount(0, 10)).toBe(1)
  })

  it('rounds up to cover the remainder', () => {
    expect(followUpRecipientPageCount(25, 10)).toBe(3)
    expect(followUpRecipientPageCount(20, 10)).toBe(2)
  })
})
