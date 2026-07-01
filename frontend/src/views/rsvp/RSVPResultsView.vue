<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { PageHeader, DataTable, type Column } from '@/components/shared'
import { rsvpService } from '@/services/api'
import { formatDateDDMMYYYY } from '@/lib/utils'
import { BarChart3, Download } from 'lucide-vue-next'

interface RSVPRow { id: string; phone_number: string; attendance: string; answers?: Record<string, unknown>; responded_at?: string; contact?: { profile_name?: string } }

const { t } = useI18n()
const route = useRoute()
const id = route.params.id as string

const tally = ref<Record<string, number>>({ yes: 0, no: 0, maybe: 0, pending: 0, total: 0 })
const responses = ref<RSVPRow[]>([])
const isLoading = ref(true)
let timer: number | undefined
const cards = ['yes', 'no', 'maybe', 'pending', 'total']

const columns = computed<Column<RSVPRow>[]>(() => [
  { key: 'guest', label: t('rsvp.guest') },
  { key: 'attendance', label: t('rsvp.status') },
  { key: 'answers', label: t('rsvp.description') },
  { key: 'responded_at', label: t('rsvp.respondedAt') },
])

async function loadTally() {
  const r = await rsvpService.tally(id)
  tally.value = (r.data as any).data || r.data
}
async function loadResponses() {
  const r = await rsvpService.responses(id)
  const d = (r.data as any).data || r.data
  responses.value = d.responses || []
}
function answerText(row: RSVPRow): string {
  const a = row.answers || {}
  return Object.entries(a)
    .filter(([k]) => !k.startsWith('_'))
    .map(([k, v]) => `${k}: ${v}`)
    .join(', ')
}
function exportXlsx() { window.open(rsvpService.exportUrl(id), '_blank') }

onMounted(async () => {
  isLoading.value = true
  try {
    await Promise.all([loadTally(), loadResponses()])
  } finally {
    isLoading.value = false
  }
  timer = window.setInterval(() => { loadTally(); loadResponses() }, 15000)
})
onUnmounted(() => { if (timer) window.clearInterval(timer) })
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="t('rsvp.resultsTitle')" :icon="BarChart3" back-link="/rsvp">
      <template #actions>
        <Button variant="outline" size="sm" @click="exportXlsx">
          <Download class="h-4 w-4 mr-2" />
          {{ t('rsvp.export') }}
        </Button>
      </template>
    </PageHeader>

    <ScrollArea class="flex-1">
      <div class="p-6">
        <div class="max-w-6xl mx-auto space-y-6">
          <div class="grid grid-cols-2 md:grid-cols-5 gap-4">
            <Card v-for="k in cards" :key="k">
              <CardHeader><CardTitle class="text-sm text-muted-foreground">{{ t('rsvp.' + k) }}</CardTitle></CardHeader>
              <CardContent><div class="text-2xl font-bold">{{ tally[k] ?? 0 }}</div></CardContent>
            </Card>
          </div>

          <Card>
            <CardContent class="pt-6">
              <DataTable
                :items="responses"
                :columns="columns"
                :is-loading="isLoading"
                :empty-icon="BarChart3"
                :empty-title="t('rsvp.noResponses')"
                item-name="responses"
              >
                <template #cell-guest="{ item }">
                  <span class="font-medium">{{ item.contact?.profile_name || item.phone_number }}</span>
                </template>
                <template #cell-attendance="{ item }">
                  {{ t('rsvp.' + item.attendance) !== 'rsvp.' + item.attendance ? t('rsvp.' + item.attendance) : item.attendance }}
                </template>
                <template #cell-answers="{ item }">
                  <span class="text-sm text-muted-foreground">{{ answerText(item) || '—' }}</span>
                </template>
                <template #cell-responded_at="{ item }">
                  <span class="text-sm text-muted-foreground">{{ item.responded_at ? formatDateDDMMYYYY(item.responded_at) : '—' }}</span>
                </template>
              </DataTable>
            </CardContent>
          </Card>
        </div>
      </div>
    </ScrollArea>
  </div>
</template>
