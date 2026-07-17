import { describe, it, expect } from 'vitest'
import { contributorFlagCount, type RSVPContributor } from './contributorFlags'

function contributor(overrides: Partial<RSVPContributor> = {}): RSVPContributor {
  return {
    label: 'Children',
    answer_key: 'children_count',
    mode: 'numeric',
    people: 0,
    responses: 0,
    needs_review: 0,
    unparseable: 0,
    ...overrides,
  }
}

describe('contributorFlagCount', () => {
  it('is zero when nothing needs checking', () => {
    expect(contributorFlagCount(contributor())).toBe(0)
  })

  it('counts unparseable answers', () => {
    expect(contributorFlagCount(contributor({ unparseable: 2 }))).toBe(2)
  })

  it('counts needs_review answers', () => {
    expect(contributorFlagCount(contributor({ needs_review: 3 }))).toBe(3)
  })

  it('sums both flags rather than picking one', () => {
    // A family answering "maybe two or three" is both ambiguous (needs_review)
    // and not a clean number (unparseable) - both must count, not just one.
    expect(contributorFlagCount(contributor({ needs_review: 1, unparseable: 2 }))).toBe(3)
  })
})
