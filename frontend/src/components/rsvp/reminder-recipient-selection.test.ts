import { ref } from 'vue'
import { describe, expect, it } from 'vitest'
import { useReminderRecipientSelection } from './reminder-recipient-selection'

describe('reminder recipient selection', () => {
  it('selects and clears the full result set independently of the visible page', () => {
    const total = ref(1033)
    const selection = useReminderRecipientSelection(total)

    expect(selection.selectedCount.value).toBe(1033)
    expect(selection.isSelected('page-1-record')).toBe(true)
    expect(selection.isSelected('page-104-record')).toBe(true)

    selection.clearAll()
    expect(selection.selectedCount.value).toBe(0)
    expect(selection.isSelected('page-1-record')).toBe(false)
    expect(selection.isSelected('page-104-record')).toBe(false)

    selection.toggle('page-104-record')
    expect(selection.selectedCount.value).toBe(1)
    expect(selection.includedIds.value).toEqual(new Set(['page-104-record']))

    selection.selectAll()
    selection.toggle('page-1-record')
    expect(selection.selectedCount.value).toBe(1032)
    expect(selection.excludedIds.value).toEqual(new Set(['page-1-record']))
    expect(selection.isSelected('page-104-record')).toBe(true)
  })
})
