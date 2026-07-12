<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from '@/components/ui/dialog'
import {
  Select, SelectTrigger, SelectValue, SelectContent, SelectItem,
} from '@/components/ui/select'
import { PageHeader, DataTable, type Column } from '@/components/shared'
import { rsvpService } from '@/services/api'
import { formatDateTimeIST } from '@/lib/utils'
import { BarChart3, Download, Pencil } from 'lucide-vue-next'

interface RSVPRow { id: string; phone_number: string; attendance: string; answers?: Record<string, unknown>; notes?: string; responded_at?: string; contact?: { profile_name?: string } }

const { t } = useI18n()
const route = useRoute()
const id = route.params.id as string

const tally = ref<Record<string, number>>({ yes: 0, no: 0, maybe: 0, pending: 0, total: 0 })
const responses = ref<RSVPRow[]>([])
const isLoading = ref(true)
let timer: number | undefined
const cards = ['yes', 'no', 'maybe', 'pending', 'total']
const attendanceOptions = ['pending', 'yes', 'no', 'maybe']

const columns = computed<Column<RSVPRow>[]>(() => [
  { key: 'name', label: t('rsvp.name') },
  { key: 'mobile', label: 'Mobile' },
  { key: 'attendance', label: t('rsvp.status') },
  { key: 'answers', label: t('rsvp.description') },
  { key: 'notes', label: t('rsvp.notes') },
  { key: 'responded_at', label: t('rsvp.respondedAt') },
  { key: 'actions', label: '' },
])

// --- Edit dialog state ---
const editOpen = ref(false)
const isSaving = ref(false)
const editingId = ref<string | null>(null)
const editAttendance = ref('pending')
const editNotes = ref('')
const editAnswers = ref<{ key: string; value: string }[]>([])

function openEdit(row: RSVPRow) {
  editingId.value = row.id
  editAttendance.value = row.attendance || 'pending'
  editNotes.value = row.notes || ''
  editAnswers.value = Object.entries(row.answers || {})
    .filter(([k]) => !k.startsWith('_'))
    .map(([key, value]) => ({ key, value: value == null ? '' : String(value) }))
  editOpen.value = true
}

async function saveEdit() {
  if (!editingId.value) return
  isSaving.value = true
  try {
    const answers: Record<string, unknown> = {}
    for (const { key, value } of editAnswers.value) {
      if (key.trim()) answers[key] = value
    }
    await rsvpService.updateResponse(id, editingId.value, {
      attendance: editAttendance.value,
      answers,
      notes: editNotes.value,
    })
    toast.success(t('rsvp.responseUpdated'))
    editOpen.value = false
    await Promise.all([loadTally(), loadResponses()])
  } catch (error: any) {
    toast.error(error?.response?.data?.message || t('rsvp.responseUpdateFailed'))
  } finally {
    isSaving.value = false
  }
}

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
function attendanceLabel(v: string): string {
  const key = 'rsvp.' + v
  return t(key) !== key ? t(key) : v
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
                <template #cell-name="{ item }">
                  <span class="font-medium">{{ item.contact?.profile_name || '—' }}</span>
                </template>
                <template #cell-mobile="{ item }">
                  <span class="text-sm">{{ item.phone_number }}</span>
                </template>
                <template #cell-attendance="{ item }">
                  {{ attendanceLabel(item.attendance) }}
                </template>
                <template #cell-answers="{ item }">
                  <span class="text-sm text-muted-foreground">{{ answerText(item) || '—' }}</span>
                </template>
                <template #cell-notes="{ item }">
                  <span class="text-sm text-muted-foreground">{{ item.notes || '—' }}</span>
                </template>
                <template #cell-responded_at="{ item }">
                  <span class="text-sm text-muted-foreground">{{ item.responded_at ? formatDateTimeIST(item.responded_at) : '—' }}</span>
                </template>
                <template #cell-actions="{ item }">
                  <Button variant="ghost" size="icon" class="h-8 w-8" @click="openEdit(item)" :title="t('rsvp.editResponse')">
                    <Pencil class="h-4 w-4" />
                  </Button>
                </template>
              </DataTable>
            </CardContent>
          </Card>
        </div>
      </div>
    </ScrollArea>

    <Dialog v-model:open="editOpen">
      <DialogContent class="max-w-md">
        <DialogHeader>
          <DialogTitle>{{ t('rsvp.editResponse') }}</DialogTitle>
        </DialogHeader>
        <div class="space-y-4 py-2">
          <div class="space-y-1.5">
            <Label>{{ t('rsvp.status') }}</Label>
            <Select v-model="editAttendance">
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem v-for="opt in attendanceOptions" :key="opt" :value="opt">
                  {{ attendanceLabel(opt) }}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div v-if="editAnswers.length" class="space-y-2">
            <Label>{{ t('rsvp.answersLabel') }}</Label>
            <div v-for="(ans, idx) in editAnswers" :key="idx" class="flex items-center gap-2">
              <span class="text-sm text-muted-foreground w-32 truncate">{{ ans.key }}</span>
              <Input v-model="editAnswers[idx].value" class="h-8 flex-1" />
            </div>
          </div>

          <div class="space-y-1.5">
            <Label>{{ t('rsvp.notes') }}</Label>
            <Textarea v-model="editNotes" :placeholder="t('rsvp.notesPlaceholder')" :rows="3" />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" @click="editOpen = false" :disabled="isSaving">{{ t('common.cancel') }}</Button>
          <Button @click="saveEdit" :disabled="isSaving">{{ isSaving ? t('common.saving') + '...' : t('common.save') }}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
