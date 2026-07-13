<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter, DialogDescription,
} from '@/components/ui/dialog'
import {
  Select, SelectTrigger, SelectValue, SelectContent, SelectItem,
} from '@/components/ui/select'
import { PageHeader, DataTable, SearchInput, type Column } from '@/components/shared'
import { rsvpService } from '@/services/api'
import { formatDateTimeIST } from '@/lib/utils'
import { BarChart3, Download, Pencil, Trash2, Send } from 'lucide-vue-next'

interface RSVPRow { id: string; phone_number: string; attendance: string; answers?: Record<string, unknown>; notes?: string; responded_at?: string; reprompted_at?: string; contact?: { profile_name?: string } }

const { t } = useI18n()
const route = useRoute()
const id = route.params.id as string

const tally = ref<Record<string, number>>({ yes: 0, no: 0, maybe: 0, pending: 0, total: 0 })
const breakdowns = ref<Record<string, Record<string, number>>>({})
const attendanceField = ref('attendance')
const selected = ref<Record<string, string>>({}) // title field -> value (member + spouse combine as AND)
const responses = ref<RSVPRow[]>([])
const isLoading = ref(true)
const searchQuery = ref('')
const page = ref(1)
const pageSize = 20
const total = ref(0)
let searchTimer: number | undefined
let timer: number | undefined
const attendanceOptions = ['pending', 'yes', 'no', 'maybe']

interface Bucket { label: string; field: string; value: string; count: number }
interface CardGroup { title: string; field: string; buckets: Bucket[] }

// Build a clickable card group for a "*_title" answer field: one card per actual
// value (dynamic) plus a Pending card for unanswered guests.
function buildGroup(title: string, field: string): CardGroup {
  const map = breakdowns.value[field] || {}
  const buckets: Bucket[] = Object.entries(map)
    .sort((a, b) => b[1] - a[1])
    .map(([value, count]) => ({ label: value, field, value, count }))
  const answered = buckets.reduce((s, b) => s + b.count, 0)
  buckets.push({ label: t('rsvp.pending'), field, value: '__pending__', count: Math.max((tally.value.total || 0) - answered, 0) })
  return { title, field, buckets }
}

const memberGroup = computed(() => buildGroup(t('rsvp.memberAttendance'), attendanceField.value + '_title'))
const spouseGroup = computed(() => {
  const key = Object.keys(breakdowns.value).find(k => k.includes('spouse')) || 'spouse_attendance_title'
  return buildGroup(t('rsvp.spouseAttendance'), key)
})
const cardGroups = computed<CardGroup[]>(() => [memberGroup.value, spouseGroup.value])

function isActive(field: string, value: string) { return selected.value[field] === value }
function toggleCardFilter(field: string, value: string) {
  const next = { ...selected.value }
  if (next[field] === value) delete next[field]
  else next[field] = value
  selected.value = next
  page.value = 1
  loadResponses()
}
function clearFilter() { selected.value = {}; page.value = 1; loadResponses() }
const hasFilter = computed(() => Object.keys(selected.value).length > 0)

// Active-filter chips (one per selected group) shown above the table.
const activeChips = computed(() =>
  cardGroups.value
    .filter(g => selected.value[g.field] != null)
    .map(g => {
      const v = selected.value[g.field]
      return { field: g.field, value: v, group: g.title, label: v === '__pending__' ? t('rsvp.pending') : v }
    }),
)

// Semantic colour for a bucket value: attending=green, not-attending=red, pending=amber.
function bucketTone(value: string): 'yes' | 'no' | 'pending' | 'neutral' {
  if (value === '__pending__') return 'pending'
  const v = value.toLowerCase()
  if (v.includes('not') || v === 'no') return 'no'
  if (v.includes('attend') || v.includes('yes')) return 'yes'
  return 'neutral'
}
const TONE_TEXT: Record<string, string> = { yes: 'text-emerald-500', no: 'text-rose-500', pending: 'text-amber-500', neutral: 'text-foreground' }
const TONE_DOT: Record<string, string> = { yes: 'bg-emerald-500', no: 'bg-rose-500', pending: 'bg-amber-500', neutral: 'bg-muted-foreground' }
function toneText(value: string) { return TONE_TEXT[bucketTone(value)] }
function toneDot(value: string) { return TONE_DOT[bucketTone(value)] }

// Union of answer keys across all responses (first-seen order), one column each.
const answerKeys = computed<string[]>(() => {
  const seen: string[] = []
  for (const row of responses.value) {
    for (const k of Object.keys(row.answers || {})) {
      if (!k.startsWith('_') && !seen.includes(k)) seen.push(k)
    }
  }
  return seen
})

function prettyKey(k: string): string {
  return k.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase())
}

const columns = computed<Column<RSVPRow>[]>(() => [
  { key: 'name', label: t('rsvp.name') },
  { key: 'mobile', label: 'Mobile' },
  { key: 'attendance', label: t('rsvp.status') },
  ...answerKeys.value.map(k => ({ key: `answers.${k}`, label: prettyKey(k) })),
  { key: 'notes', label: t('rsvp.notes') },
  { key: 'responded_at', label: t('rsvp.respondedAt') },
  { key: 'reprompted', label: t('rsvp.reprompted') },
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

// --- Delete dialog state ---
const deleteOpen = ref(false)
const isDeleting = ref(false)
const deletingRow = ref<RSVPRow | null>(null)

function openDelete(row: RSVPRow) {
  deletingRow.value = row
  deleteOpen.value = true
}

async function confirmDelete() {
  if (!deletingRow.value) return
  isDeleting.value = true
  try {
    await rsvpService.deleteResponse(id, deletingRow.value.id)
    toast.success(t('rsvp.responseDeleted'))
    deleteOpen.value = false
    deletingRow.value = null
    await Promise.all([loadTally(), loadResponses()])
  } catch (error: any) {
    toast.error(error?.response?.data?.message || t('rsvp.responseDeleteFailed'))
  } finally {
    isDeleting.value = false
  }
}

async function loadTally() {
  const r = await rsvpService.tally(id)
  const d = (r.data as any).data || r.data
  tally.value = { yes: d.yes || 0, no: d.no || 0, maybe: d.maybe || 0, pending: d.pending || 0, total: d.total || 0 }
  breakdowns.value = d.breakdowns || {}
  attendanceField.value = d.attendance_field || 'attendance'
}
async function loadResponses() {
  const pairs = Object.entries(selected.value)
  const r = await rsvpService.responses(id, {
    search: searchQuery.value || undefined,
    page: page.value,
    limit: pageSize,
    title_field: pairs[0]?.[0],
    title_value: pairs[0]?.[1],
    title_field2: pairs[1]?.[0],
    title_value2: pairs[1]?.[1],
  })
  const d = (r.data as any).data || r.data
  responses.value = d.responses || []
  total.value = d.total || 0
}
function onSearch() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = window.setTimeout(() => { page.value = 1; loadResponses() }, 300)
}
function onPageChange(p: number) { page.value = p; loadResponses() }
function attendanceLabel(v: string): string {
  const key = 'rsvp.' + v
  return t(key) !== key ? t(key) : v
}
function exportXlsx() { window.open(rsvpService.exportUrl(id), '_blank') }

const reprompting = ref(false)
const repromptOpen = ref(false)
const repromptTargets = ref<{ phone: string; name: string; reason: string; reprompted_at?: string }[]>([])
const repromptMessage = ref('')
const repromptSearch = ref('')
const selectedPhones = ref<Set<string>>(new Set())

const filteredTargets = computed(() => {
  const q = repromptSearch.value.toLowerCase().trim()
  if (!q) return repromptTargets.value
  return repromptTargets.value.filter(t => (t.name || '').toLowerCase().includes(q) || (t.phone || '').includes(q))
})

function toggleTarget(phone: string) {
  const s = new Set(selectedPhones.value)
  if (s.has(phone)) s.delete(phone); else s.add(phone)
  selectedPhones.value = s
}
function toggleAllVisible(check: boolean) {
  const s = new Set(selectedPhones.value)
  filteredTargets.value.forEach(t => { if (check) s.add(t.phone); else s.delete(t.phone) })
  selectedPhones.value = s
}

// Preview who will be messaged and what, before sending.
async function reprompt() {
  reprompting.value = true
  try {
    const r = await rsvpService.repromptPreview(id)
    const d = (r.data as any)?.data || r.data
    repromptTargets.value = d.targets || []
    repromptMessage.value = d.message || ''
    repromptSearch.value = ''
    selectedPhones.value = new Set(repromptTargets.value.map(t => t.phone))
    repromptOpen.value = true
  } catch (error: any) {
    toast.error(error?.response?.data?.message || t('rsvp.repromptFailed'))
  } finally {
    reprompting.value = false
  }
}

async function confirmReprompt() {
  reprompting.value = true
  try {
    const r = await rsvpService.reprompt(id, [...selectedPhones.value])
    const n = (r.data as any)?.data?.reprompted ?? (r.data as any)?.reprompted ?? 0
    toast.success(t('rsvp.repromptSent', { count: n }))
    repromptOpen.value = false
    await Promise.all([loadTally(), loadResponses()])
  } catch (error: any) {
    toast.error(error?.response?.data?.message || t('rsvp.repromptFailed'))
  } finally {
    reprompting.value = false
  }
}

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
        <Button variant="outline" size="sm" :disabled="reprompting" @click="reprompt">
          <Send class="h-4 w-4 mr-2" />
          {{ t('rsvp.reprompt') }}
        </Button>
        <Button variant="outline" size="sm" @click="exportXlsx">
          <Download class="h-4 w-4 mr-2" />
          {{ t('rsvp.export') }}
        </Button>
      </template>
    </PageHeader>

    <ScrollArea class="flex-1">
      <div class="p-6">
        <div class="max-w-6xl mx-auto space-y-6">
          <div class="space-y-4">
            <!-- Total + active filter chips -->
            <div class="flex flex-wrap items-center gap-3">
              <button
                type="button"
                @click="clearFilter"
                :class="['rounded-xl border px-5 py-3 text-left transition min-w-[130px]', !hasFilter ? 'border-primary bg-primary/5 ring-1 ring-primary' : 'border-border hover:bg-muted/40']"
              >
                <div class="text-xs font-medium text-muted-foreground">{{ t('rsvp.total') }}</div>
                <div class="text-3xl font-bold tabular-nums">{{ tally.total }}</div>
              </button>
              <div v-if="hasFilter" class="flex flex-wrap items-center gap-2">
                <span
                  v-for="c in activeChips"
                  :key="c.field"
                  class="inline-flex items-center gap-1.5 rounded-full bg-primary/10 text-primary text-xs font-medium px-3 py-1.5"
                >
                  {{ c.group }}: {{ c.label }}
                  <button type="button" class="hover:text-primary/60" @click="toggleCardFilter(c.field, c.value)">✕</button>
                </span>
                <button type="button" class="text-xs text-muted-foreground hover:underline" @click="clearFilter">{{ t('rsvp.clearFilter') }}</button>
              </div>
            </div>

            <!-- Member / Spouse groups -->
            <div class="grid gap-4 md:grid-cols-2">
              <div v-for="grp in cardGroups" :key="grp.field" class="rounded-xl border bg-card p-4">
                <div class="mb-3 text-sm font-semibold">{{ grp.title }}</div>
                <div class="grid grid-cols-3 gap-2">
                  <button
                    v-for="b in grp.buckets"
                    :key="b.value"
                    type="button"
                    @click="toggleCardFilter(b.field, b.value)"
                    :class="['rounded-lg border p-3 text-left transition-all', isActive(b.field, b.value) ? 'border-primary bg-primary/5 ring-1 ring-primary' : 'border-border hover:bg-muted/40']"
                  >
                    <div class="flex items-center gap-1.5">
                      <span :class="['h-2 w-2 shrink-0 rounded-full', toneDot(b.value)]"></span>
                      <span class="text-xs font-medium text-muted-foreground truncate">{{ b.label }}</span>
                    </div>
                    <div :class="['mt-1 text-2xl font-bold tabular-nums', toneText(b.value)]">{{ b.count }}</div>
                  </button>
                </div>
              </div>
            </div>
          </div>

          <Card>
            <CardContent class="pt-6">
              <div class="mb-4 flex justify-end">
                <SearchInput v-model="searchQuery" :placeholder="t('rsvp.searchResponses')" class="w-72" @update:model-value="onSearch" />
              </div>
              <DataTable
                :items="responses"
                :columns="columns"
                :is-loading="isLoading"
                :empty-icon="BarChart3"
                :empty-title="t('rsvp.noResponses')"
                item-name="responses"
                server-pagination
                :current-page="page"
                :total-items="total"
                :page-size="pageSize"
                @page-change="onPageChange"
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
                <template #cell-notes="{ item }">
                  <span class="text-sm text-muted-foreground">{{ item.notes || '—' }}</span>
                </template>
                <template #cell-responded_at="{ item }">
                  <span class="text-sm text-muted-foreground">{{ item.responded_at ? formatDateTimeIST(item.responded_at) : '—' }}</span>
                </template>
                <template #cell-reprompted="{ item }">
                  <span v-if="item.reprompted_at" class="text-xs text-blue-600" :title="formatDateTimeIST(item.reprompted_at)">✓ {{ formatDateTimeIST(item.reprompted_at) }}</span>
                  <span v-else class="text-sm text-muted-foreground">—</span>
                </template>
                <template #cell-actions="{ item }">
                  <div class="flex items-center justify-end gap-1">
                    <Button variant="ghost" size="icon" class="h-8 w-8" @click="openEdit(item)" :title="t('rsvp.editResponse')">
                      <Pencil class="h-4 w-4" />
                    </Button>
                    <Button variant="ghost" size="icon" class="h-8 w-8 text-destructive" @click="openDelete(item)" :title="t('rsvp.deleteResponse')">
                      <Trash2 class="h-4 w-4" />
                    </Button>
                  </div>
                </template>
              </DataTable>
            </CardContent>
          </Card>
        </div>
      </div>
    </ScrollArea>

    <Dialog v-model:open="repromptOpen">
      <DialogContent class="max-w-lg">
        <DialogHeader>
          <DialogTitle>{{ t('rsvp.reprompt') }}</DialogTitle>
          <DialogDescription>{{ t('rsvp.repromptConfirm', { count: repromptTargets.length }) }}</DialogDescription>
        </DialogHeader>
        <div class="space-y-3 py-2">
          <div class="space-y-1">
            <Label class="text-xs">{{ t('rsvp.repromptMessage') }}</Label>
            <div class="text-xs bg-muted/40 rounded p-2 max-h-32 overflow-y-auto whitespace-pre-wrap">{{ repromptMessage || '—' }}</div>
          </div>
          <div class="space-y-1">
            <div class="flex items-center justify-between">
              <Label class="text-xs">{{ t('rsvp.repromptRecipients') }} ({{ selectedPhones.size }}/{{ repromptTargets.length }})</Label>
              <div class="flex items-center gap-2">
                <button type="button" class="text-[10px] text-primary hover:underline" @click="toggleAllVisible(true)">{{ t('rsvp.selectAll') }}</button>
                <button type="button" class="text-[10px] text-primary hover:underline" @click="toggleAllVisible(false)">{{ t('rsvp.clearSelection') }}</button>
              </div>
            </div>
            <Input v-model="repromptSearch" :placeholder="t('rsvp.searchResponses')" class="h-7 text-xs" />
            <div class="border rounded max-h-56 overflow-y-auto divide-y">
              <label v-for="(tg, i) in filteredTargets" :key="i" class="flex items-center gap-2 px-2 py-1 text-xs cursor-pointer">
                <input type="checkbox" :checked="selectedPhones.has(tg.phone)" @change="toggleTarget(tg.phone)" />
                <span class="flex-1">{{ tg.name || '—' }} <span class="text-muted-foreground">{{ tg.phone }}</span></span>
                <span v-if="tg.reprompted_at" class="text-[10px] text-blue-600" :title="formatDateTimeIST(tg.reprompted_at)">✓ {{ t('rsvp.alreadyReprompted') }}</span>
                <span class="text-[10px] text-muted-foreground">{{ tg.reason }}</span>
              </label>
              <div v-if="!filteredTargets.length" class="px-2 py-3 text-center text-xs text-muted-foreground">{{ t('rsvp.noResponses') }}</div>
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" @click="repromptOpen = false" :disabled="reprompting">{{ t('common.cancel') }}</Button>
          <Button @click="confirmReprompt" :disabled="reprompting || !selectedPhones.size">
            {{ t('rsvp.repromptSendNow', { count: selectedPhones.size }) }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

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
              <span class="text-sm text-muted-foreground w-32 truncate">{{ prettyKey(ans.key) }}</span>
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

    <Dialog v-model:open="deleteOpen">
      <DialogContent class="max-w-sm">
        <DialogHeader>
          <DialogTitle>{{ t('rsvp.deleteResponse') }}</DialogTitle>
          <DialogDescription>
            {{ t('rsvp.deleteResponseConfirm', { name: deletingRow?.contact?.profile_name || deletingRow?.phone_number || '' }) }}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" @click="deleteOpen = false" :disabled="isDeleting">{{ t('common.cancel') }}</Button>
          <Button variant="destructive" @click="confirmDelete" :disabled="isDeleting">
            {{ t('common.delete') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
