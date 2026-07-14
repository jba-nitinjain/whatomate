import { computed, ref, type Ref } from 'vue'

export function useReminderRecipientSelection(total: Ref<number>) {
  const allSelected = ref(true)
  const excludedIds = ref<Set<string>>(new Set())
  const includedIds = ref<Set<string>>(new Set())

  const selectedCount = computed(() => allSelected.value
    ? Math.max(0, total.value - excludedIds.value.size)
    : includedIds.value.size)

  function selectAll() {
    allSelected.value = true
    excludedIds.value = new Set()
    includedIds.value = new Set()
  }

  function clearAll() {
    allSelected.value = false
    excludedIds.value = new Set()
    includedIds.value = new Set()
  }

  function isSelected(id: string) {
    return allSelected.value ? !excludedIds.value.has(id) : includedIds.value.has(id)
  }

  function toggle(id: string) {
    const next = new Set(allSelected.value ? excludedIds.value : includedIds.value)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    if (allSelected.value) excludedIds.value = next
    else includedIds.value = next
  }

  return {
    allSelected,
    excludedIds,
    includedIds,
    selectedCount,
    selectAll,
    clearAll,
    isSelected,
    toggle,
  }
}
