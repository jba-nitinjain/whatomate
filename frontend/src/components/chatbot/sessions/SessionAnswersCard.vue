<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { formatLabel } from '@/lib/utils'
import { ClipboardList } from 'lucide-vue-next'

const props = defineProps<{ data: Record<string, unknown> | null | undefined }>()
const { t } = useI18n()

// Keys starting with "_" are the engine's internal bookkeeping (flow id, RSVP markers).
const answers = computed(() =>
  Object.entries(props.data || {})
    .filter(([key]) => !key.startsWith('_'))
    .map(([key, value]) => ({ key, label: formatLabel(key), value: formatValue(value) }))
)

function formatValue(value: unknown): string {
  if (value === null || value === undefined || value === '') return '—'
  if (typeof value === 'boolean') return value ? t('common.yes', 'Yes') : t('common.no', 'No')
  if (Array.isArray(value)) return value.map(v => (typeof v === 'object' ? JSON.stringify(v) : String(v))).join(', ') || '—'
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}
</script>

<template>
  <Card>
    <CardHeader class="pb-3">
      <div class="flex items-center gap-2">
        <ClipboardList class="h-4 w-4 text-muted-foreground" />
        <CardTitle class="text-sm font-medium">{{ t('chatbotSessions.answers') }}</CardTitle>
      </div>
      <CardDescription>{{ t('chatbotSessions.answersDesc') }}</CardDescription>
    </CardHeader>
    <CardContent>
      <dl v-if="answers.length" class="grid grid-cols-1 sm:grid-cols-2 gap-3" data-testid="session-answers">
        <div v-for="item in answers" :key="item.key" class="rounded-md bg-muted/50 px-3 py-2.5">
          <dt class="text-xs text-muted-foreground">{{ item.label }}</dt>
          <dd class="mt-0.5 text-sm font-medium break-words whitespace-pre-wrap">{{ item.value }}</dd>
        </div>
      </dl>
      <p v-else class="text-sm text-muted-foreground">{{ t('chatbotSessions.noAnswers') }}</p>
    </CardContent>
  </Card>
</template>
