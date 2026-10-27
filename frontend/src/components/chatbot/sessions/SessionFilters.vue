<script setup lang="ts">
import { watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { SearchInput, DateRangePicker } from '@/components/shared'
import { useDateRange, type DateRangeResult } from '@/composables/useDateRange'

defineProps<{ flows: { id: string; name: string }[] }>()

const status = defineModel<string>('status', { default: 'all' })
const flowId = defineModel<string>('flowId', { default: 'all' })
const search = defineModel<string>('search', { default: '' })
const range = defineModel<DateRangeResult>('range')

const { t } = useI18n()

const STATUSES = ['active', 'completed', 'timeout', 'cancelled'] as const

const {
  selectedRange, customDateRange, isDatePickerOpen,
  dateRange, formatDateRangeDisplay, applyCustomRange: baseApplyCustomRange,
} = useDateRange({ defaultPreset: '30days', storageKey: 'chatbot_sessions' })

// Publish the initial range, then only on preset change or an applied custom range
// (picking dates in the calendar must not refetch before "Apply").
range.value = dateRange.value

watch(selectedRange, (val) => {
  if (val !== 'custom') range.value = dateRange.value
})

function applyCustomRange() {
  baseApplyCustomRange()
  range.value = dateRange.value
}
</script>

<template>
  <div class="flex items-center gap-2 flex-wrap">
    <SearchInput
      v-model="search"
      :placeholder="t('chatbotSessions.searchPlaceholder')"
      class="w-full sm:w-56"
      data-testid="sessions-search"
    />
    <Select v-model="status">
      <SelectTrigger class="w-[150px]" data-testid="sessions-status-filter">
        <SelectValue :placeholder="t('chatbotSessions.allStatuses')" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">{{ t('chatbotSessions.allStatuses') }}</SelectItem>
        <SelectItem v-for="s in STATUSES" :key="s" :value="s">
          {{ t(`chatbotSessions.status_${s}`) }}
        </SelectItem>
      </SelectContent>
    </Select>
    <Select v-model="flowId">
      <SelectTrigger class="w-[180px]" data-testid="sessions-flow-filter">
        <SelectValue :placeholder="t('chatbotSessions.allFlows')" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">{{ t('chatbotSessions.allFlows') }}</SelectItem>
        <SelectItem v-for="flow in flows" :key="flow.id" :value="flow.id">
          {{ flow.name }}
        </SelectItem>
      </SelectContent>
    </Select>
    <DateRangePicker
      v-model:selected-range="selectedRange"
      v-model:custom-date-range="customDateRange"
      v-model:is-date-picker-open="isDatePickerOpen"
      :format-date-range-display="formatDateRangeDisplay"
      @apply-custom="applyCustomRange"
    />
  </div>
</template>
