<script setup lang="ts">
import { ref, onMounted, onUnmounted, onErrorCaptured, computed } from 'vue'
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
import RSVPGuestManagerDialog from '@/components/rsvp/RSVPGuestManagerDialog.vue'
import RSVPReminderDialog from '@/components/rsvp/RSVPReminderDialog.vue'
import RSVPFollowUpDialog from '@/components/rsvp/RSVPFollowUpDialog.vue'
import { rsvpService } from '@/services/api'
import { formatDateTimeIST } from '@/lib/utils'
import { BarChart3, Bell, Download, Mail, Pencil, Trash2, Send, DownloadCloud, Users, MessageCirclePlus } from 'lucide-vue-next'
import { visibleAnswerKeys } from './answerColumns'
import { contributorFlagCount, type RSVPContributor } from './contributorFlags'
import { resolveAnswerColumnLabel, resolveContributorCardLabel } from './answerLabels'

interface RSVPRow { id: string; contact_id: string; phone_number: string; attendance: string; source: string; journey_status: string; invite_sent_at?: string; reminder_count: number; last_reminder_at?: string; rsvp_started_at?: string; answers?: Record<string, unknown>; notes?: string; responded_at?: string; reprompted_at?: string; contact?: { profile_name?: string } }
interface AttendanceCounts { attending: number; not_attending: number; maybe: number; pending: number }
interface RSVPTally extends AttendanceCounts { yes: number; no: number; total: number; member_attendance: AttendanceCounts; spouse_attendance: AttendanceCounts; total_attending: number; contributors: RSVPContributor[] }

const { t } = useI18n()
const route = useRoute()
const id = route.params.id as string

const emptyAttendance = (): AttendanceCounts => ({ attending: 0, not_attending: 0, maybe: 0, pending: 0 })
const tally = ref<RSVPTally>({ yes: 0, no: 0, total: 0, ...emptyAttendance(), member_attendance: emptyAttendance(), spouse_attendance: emptyAttendance(), total_attending: 0, contributors: [] })
const attendanceField = ref('attendance')
const responses = ref<RSVPRow[]>([])
const isLoading = ref(true)
const searchQuery = ref('')
const page = ref(1)
const pageSize = 20
const total = ref(0)
const journeyCounts = ref({ not_started: 0, in_progress: 0, responded: 0 })
const journeyFilter = ref('')
const memberFilter = ref('')
const spouseFilter = ref('')
const selectedGuestIds = ref<Set<string>>(new Set())
const guestManagerOpen = ref(false)
const reminderManagerOpen = ref(false)
const reminderRenderError = ref('')
const followUpOpen = ref(false)
let searchTimer: number | undefined
let timer: number | undefined
const attendanceOptions = ['pending', 'yes', 'no', 'maybe']

function openReminderManager() {
  reminderRenderError.value = ''
  reminderManagerOpen.value = true
}

function closeReminderManager() {
  reminderManagerOpen.value = false
  reminderRenderError.value = ''
}

onErrorCaptured((error) => {
  if (!reminderManagerOpen.value) return
  reminderRenderError.value = error instanceof Error ? error.message : String(error)
  return false
})

interface Bucket { label: string; field: 'member_status' | 'spouse_status'; value: string; count: number }
interface CardGroup { title: string; field: string; buckets: Bucket[] }

function buildGroup(title: string, field: 'member_status' | 'spouse_status', counts: AttendanceCounts): CardGroup {
  const buckets: Bucket[] = [
    { label: t('rsvp.yes'), field, value: 'attending', count: counts.attending },
    { label: t('rsvp.no'), field, value: 'not_attending', count: counts.not_attending },
  ]
  if (counts.maybe > 0) buckets.push({ label: t('rsvp.maybe'), field, value: 'maybe', count: counts.maybe })
  buckets.push({ label: t('rsvp.pending'), field, value: 'pending', count: counts.pending })
  return { title, field, buckets }
}

const memberGroup = computed(() => buildGroup(t('rsvp.memberAttendance'), 'member_status', tally.value.member_attendance))
const spouseGroup = computed(() => buildGroup(t('rsvp.spouseAttendance'), 'spouse_status', tally.value.spouse_attendance))
const cardGroups = computed<CardGroup[]>(() => [memberGroup.value, spouseGroup.value])

function isActive(field: string, value: string) { return (field === 'member_status' ? memberFilter.value : spouseFilter.value) === value }
function toggleCardFilter(field: 'member_status' | 'spouse_status', value: string) {
  const target = field === 'member_status' ? memberFilter : spouseFilter
  target.value = target.value === value ? '' : value
  page.value = 1
  loadResponses()
}

// Semantic colour for a bucket value: attending=green, not-attending=red, pending=amber.
function bucketTone(value: string): 'yes' | 'no' | 'pending' | 'neutral' {
  if (value === 'pending') return 'pending'
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
const answerKeys = computed<string[]>(() => visibleAnswerKeys(responses.value))

// Configured contributors (or the legacy member+spouse pair the API falls back
// to), used below to label a results column from its contributor label instead
// of a hardcoded "spouse_attendance" key check - a renamed spouse question used
// to fall through to the generic underscore-replacement prettifier with no
// warning.
const contributors = computed<RSVPContributor[]>(() => tally.value.contributors)

// See answerLabels.ts for the translation-precedence rules these delegate to
// (built-in member/spouse columns always translated; a contributor's own
// label is the correct, untranslated display only for a user-authored row).
function prettyKey(k: string): string {
  const base = k.endsWith('_title') ? k.slice(0, -'_title'.length) : k
  return resolveAnswerColumnLabel(base, attendanceField.value, contributors.value, t)
}

function contributorDisplayLabel(c: RSVPContributor): string {
  return resolveContributorCardLabel(c, t)
}

const columns = computed<Column<RSVPRow>[]>(() => [
  { key: 'select', label: '' },
  { key: 'name', label: t('rsvp.name') },
  { key: 'mobile', label: 'Mobile' },
  { key: 'source', label: t('rsvp.source') },
  { key: 'invite', label: t('rsvp.inviteStatus') },
  { key: 'journey', label: t('rsvp.journey') },
  { key: 'attendance', label: t('rsvp.status') },
  ...answerKeys.value.map(k => ({ key: `answers.${k}`, label: prettyKey(k) })),
  { key: 'notes', label: t('rsvp.notes') },
  { key: 'responded_at', label: t('rsvp.respondedAt') },
  { key: 'reprompted', label: t('rsvp.reprompted') },
  { key: 'reminders', label: t('rsvp.reminders') },
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
  tally.value = {
    yes: d.yes || 0, no: d.no || 0, maybe: d.maybe || 0, pending: d.pending || 0, total: d.total || 0,
    attending: d.yes || 0, not_attending: d.no || 0,
    member_attendance: { ...emptyAttendance(), ...(d.member_attendance || {}) },
    spouse_attendance: { ...emptyAttendance(), ...(d.spouse_attendance || {}) },
    total_attending: d.total_attending || 0,
    contributors: d.contributors || [],
  }
  attendanceField.value = d.attendance_field || 'attendance'
}
async function loadResponses() {
  const r = await rsvpService.guests(id, {
    search: searchQuery.value || undefined,
    page: page.value,
    limit: pageSize,
    journey_status: journeyFilter.value || undefined,
    member_status: memberFilter.value || undefined,
    spouse_status: spouseFilter.value || undefined,
  })
  const d = (r.data as any).data || r.data
  responses.value = d.guests || []
  const visibleIds = new Set(responses.value.map(row => row.id))
  selectedGuestIds.value = new Set([...selectedGuestIds.value].filter(responseId => visibleIds.has(responseId)))
  total.value = d.total || 0
  journeyCounts.value = d.journey_counts || { not_started: 0, in_progress: 0, responded: 0 }
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

function setJourneyFilter(value: string) { journeyFilter.value = journeyFilter.value === value ? '' : value; page.value = 1; loadResponses() }
function journeyCount(value: string) { return journeyCounts.value[value as keyof typeof journeyCounts.value] || 0 }
function toggleGuest(id: string) { const next = new Set(selectedGuestIds.value); if (next.has(id)) next.delete(id); else next.add(id); selectedGuestIds.value = next }
const allVisibleSelected = computed(() => responses.value.length > 0 && responses.value.every(row => selectedGuestIds.value.has(row.id)))
const someVisibleSelected = computed(() => !allVisibleSelected.value && responses.value.some(row => selectedGuestIds.value.has(row.id)))
function toggleVisibleGuests() {
  const next = new Set(selectedGuestIds.value)
  if (allVisibleSelected.value) responses.value.forEach(row => next.delete(row.id))
  else responses.value.forEach(row => next.add(row.id))
  selectedGuestIds.value = next
}
const selectedRows = computed(() => responses.value.filter(row => selectedGuestIds.value.has(row.id)))
const selectedReminderIds = computed(() => selectedRows.value.filter(row => row.journey_status === 'not_started').map(row => row.id))
async function sendInvitations() {
  const contactIds = selectedRows.value.filter(row => !row.responded_at).map(row => row.contact_id)
  if (!contactIds.length) return
  try {
    const response = await rsvpService.sendInvites(id, contactIds)
    const data = (response.data as any).data || response.data
    toast.success(t('rsvp.inviteResult', { sent: data.sent || 0, failed: data.failed || 0 }))
    selectedGuestIds.value = new Set(); await loadResponses()
  } catch (error: any) { toast.error(error?.response?.data?.message || t('rsvp.inviteFailed')) }
}

const recovering = ref(false)
// Commit partial answers left in abandoned chatbot sessions into the results.
async function recoverPartials() {
  recovering.value = true
  try {
    const r = await rsvpService.recoverPartials(id)
    const n = (r.data as any)?.data?.recovered ?? (r.data as any)?.recovered ?? 0
    toast.success(t('rsvp.recoverDone', { count: n }))
    await Promise.all([loadTally(), loadResponses()])
  } catch (error: any) {
    toast.error(error?.response?.data?.message || t('rsvp.recoverFailed'))
  } finally {
    recovering.value = false
  }
}

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
        <Button variant="outline" size="sm" @click="guestManagerOpen = true"><Users class="h-4 w-4 mr-2" />{{ t('rsvp.manageGuests') }}</Button>
        <Button variant="outline" size="sm" :disabled="!selectedRows.length" @click="sendInvitations"><Mail class="h-4 w-4 mr-2" />{{ t('rsvp.sendInvites') }}</Button>
        <Button variant="outline" size="sm" @click="openReminderManager"><Bell class="h-4 w-4 mr-2" />{{ t('rsvp.reminders') }}</Button>
        <Button variant="outline" size="sm" @click="followUpOpen = true"><MessageCirclePlus class="h-4 w-4 mr-2" />{{ t('rsvp.followUp') }}</Button>
        <Button variant="outline" size="sm" :disabled="recovering" @click="recoverPartials" :title="t('rsvp.recoverHint')">
          <DownloadCloud class="h-4 w-4 mr-2" />
          {{ t('rsvp.recover') }}
        </Button>
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
          <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
            <button type="button" class="rounded-xl border p-4 text-left" :class="!journeyFilter ? 'border-primary bg-primary/5' : ''" @click="journeyFilter = ''; loadResponses()"><div class="text-xs text-muted-foreground">{{ t('rsvp.total') }}</div><div class="text-3xl font-bold">{{ tally.total }}</div></button>
            <button v-for="status in ['not_started', 'in_progress', 'responded']" :key="status" type="button" class="rounded-xl border p-4 text-left" :class="journeyFilter === status ? 'border-primary bg-primary/5' : ''" @click="setJourneyFilter(status)"><div class="text-xs text-muted-foreground">{{ t('rsvp.' + status) }}</div><div class="text-3xl font-bold">{{ journeyCount(status) }}</div></button>
            <div class="rounded-xl border border-primary bg-primary/10 p-4 text-left">
              <div class="text-xs font-medium text-primary">{{ t('rsvp.totalAttending') }}</div>
              <div class="text-3xl font-bold text-primary">{{ tally.total_attending }}</div>
            </div>
          </div>
          <div class="space-y-4">
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
            <div v-if="tally.contributors.length" class="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
              <div v-for="c in tally.contributors" :key="c.answer_key" class="rounded-xl border bg-card p-4">
                <div class="text-xs font-medium text-muted-foreground truncate">{{ contributorDisplayLabel(c) }}</div>
                <div class="mt-1 text-2xl font-bold tabular-nums">{{ c.people }}</div>
                <div v-if="contributorFlagCount(c) > 0" class="mt-2 flex items-center gap-1 text-xs text-amber-500">
                  <span class="h-1.5 w-1.5 shrink-0 rounded-full bg-amber-500"></span>
                  {{ t('rsvp.needsChecking', { count: contributorFlagCount(c) }) }}
                </div>
              </div>
            </div>
          </div>

          <Card>
            <CardContent class="pt-6">
              <div class="mb-4 flex items-center justify-between gap-3">
                <div class="flex items-center gap-2">
                  <span class="text-sm text-muted-foreground">{{ t('rsvp.selectedGuests', { count: selectedGuestIds.size }) }}</span>
                  <Button variant="ghost" size="sm" :disabled="!responses.length" @click="toggleVisibleGuests">{{ allVisibleSelected ? t('rsvp.clearSelection') : t('rsvp.selectAll') }}</Button>
                </div>
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
                <template #header-select>
                  <input type="checkbox" :checked="allVisibleSelected" :indeterminate="someVisibleSelected" :aria-label="t('rsvp.selectAll')" @change="toggleVisibleGuests" />
                </template>
                <template #cell-select="{ item }"><input type="checkbox" :checked="selectedGuestIds.has(item.id)" @change="toggleGuest(item.id)" /></template>
                <template #cell-name="{ item }">
                  <span class="font-medium">{{ item.contact?.profile_name || '—' }}</span>
                </template>
                <template #cell-mobile="{ item }">
                  <span class="text-sm">{{ item.phone_number }}</span>
                </template>
                <template #cell-source="{ item }"><span class="text-xs">{{ t('rsvp.source_' + item.source) }}</span></template>
                <template #cell-invite="{ item }"><span class="text-xs text-muted-foreground">{{ item.invite_sent_at ? formatDateTimeIST(item.invite_sent_at) : t('rsvp.notSent') }}</span></template>
                <template #cell-journey="{ item }"><span class="text-xs font-medium">{{ t('rsvp.' + item.journey_status) }}</span></template>
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
                <template #cell-reminders="{ item }"><span class="text-xs">{{ item.reminder_count || 0 }}<span v-if="item.last_reminder_at" class="block text-muted-foreground">{{ formatDateTimeIST(item.last_reminder_at) }}</span></span></template>
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

    <RSVPGuestManagerDialog v-model:open="guestManagerOpen" :event-id="id" @changed="loadResponses" />
    <RSVPReminderDialog v-if="!reminderRenderError" v-model:open="reminderManagerOpen" :event-id="id" :selected-ids="selectedReminderIds" @changed="loadResponses" />
    <RSVPFollowUpDialog v-model:open="followUpOpen" :event-id="id" @changed="loadResponses" />
    <Teleport to="body">
      <div v-if="reminderManagerOpen && reminderRenderError" class="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4">
        <section role="alertdialog" aria-modal="true" aria-labelledby="rsvp-reminder-error-title" class="w-full max-w-lg space-y-4 rounded-lg border bg-background p-6 shadow-lg">
          <div>
            <h2 id="rsvp-reminder-error-title" class="text-lg font-semibold">{{ t('rsvp.reminderLoadFailed') }}</h2>
            <p class="mt-2 break-words rounded-md bg-destructive/10 p-3 text-sm text-destructive">{{ reminderRenderError }}</p>
          </div>
          <div class="flex justify-end"><Button variant="outline" @click="closeReminderManager">{{ t('common.close') }}</Button></div>
        </section>
      </div>
    </Teleport>

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
