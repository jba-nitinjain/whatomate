<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { rsvpService, templatesService } from '@/services/api'
import { formatDateTimeIST } from '@/lib/utils'
import { getErrorMessage } from '@/lib/api-utils'
import { responseCollection, responsePayload, templateParameterNames } from './reminder-dialog-utils'
import { useReminderRecipientSelection } from './reminder-recipient-selection'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ExternalLink, Loader2, Search, Send, Trash2, X } from 'lucide-vue-next'

interface Schedule { id: string; scheduled_at: string; template_id: string; status: string; sent_count: number; failed_count: number; campaign_id?: string }
interface Guest { id: string; phone_number: string; contact?: { profile_name?: string } }

const props = defineProps<{ open: boolean; eventId: string; selectedIds: string[] }>()
const emit = defineEmits<{ 'update:open': [value: boolean]; changed: [] }>()
const { t } = useI18n()
const router = useRouter()
const schedules = ref<Schedule[]>([])
const templates = ref<any[]>([])
const templateId = ref('')
const templateParams = ref<Record<string, string>>({})
const scheduledAt = ref('')
const loading = ref(false)
const guestLoading = ref(false)
const loadError = ref('')
const guests = ref<Guest[]>([])
const guestSearch = ref('')
const guestPage = ref(1)
const guestLimit = 10
const filteredGuestTotal = ref(0)
const recipientTotal = ref(0)
const createdCampaignId = ref('')
const createdCampaignName = ref('')
let searchTimer: number | undefined

const {
  allSelected: allRecipientsSelected,
  excludedIds,
  includedIds,
  selectedCount: includedCount,
  selectAll: selectAllRecipients,
  clearAll: clearAllRecipients,
  isSelected: isRecipientSelected,
  toggle: toggleRecipient,
} = useReminderRecipientSelection(recipientTotal)

const guestPages = computed(() => Math.max(1, Math.ceil(filteredGuestTotal.value / guestLimit)))
const selectedSendCount = computed(() => props.selectedIds.filter(isRecipientSelected).length)
const selectedTemplate = computed(() => templates.value.find(template => template.id === templateId.value))
const templateParamNames = computed(() => templateParameterNames(selectedTemplate.value))
const missingTemplateParams = computed(() => templateParamNames.value.filter(name => !String(templateParams.value[name] || '').trim()))

function syncTemplateParams() {
  const next: Record<string, string> = {}
  for (const name of templateParamNames.value) next[name] = templateParams.value[name] || ''
  templateParams.value = next
}

function formatTemplateParam(name: string) { return '{' + '{' + name + '}' + '}' }
function isRecord(value: unknown): value is Record<string, any> { return !!value && typeof value === 'object' && !Array.isArray(value) }
function isGuest(value: unknown): value is Guest { return isRecord(value) && typeof value.id === 'string' && typeof value.phone_number === 'string' }
function validSchedules(response: any) {
  return responseCollection<Schedule>(response, 'reminders').filter(item => isRecord(item) && typeof item.id === 'string' && typeof item.scheduled_at === 'string')
}
function validTemplates(response: any) { return responseCollection<any>(response, 'templates').filter(isRecord) }
function formatScheduleDate(value: unknown) { return typeof value === 'string' ? formatDateTimeIST(value) : '' }

function applyRecipientResponse(response: any) {
  const data = responsePayload(response)
  guests.value = Array.isArray(data.guests) ? data.guests.filter(isGuest) : []
  filteredGuestTotal.value = typeof data.total === 'number' ? data.total : 0
  if (!guestSearch.value) recipientTotal.value = filteredGuestTotal.value
}

function loadErrorMessage(error?: unknown) {
  const fallback = t('rsvp.reminderLoadFailed')
  return error ? getErrorMessage(error, fallback) : fallback
}

async function loadRecipients() {
  guestLoading.value = true
  try {
    const response = await rsvpService.guests(props.eventId, {
      journey_status: 'not_started', search: guestSearch.value || undefined,
      page: guestPage.value, limit: guestLimit,
    })
    applyRecipientResponse(response)
  } catch (error) {
    guests.value = []
    filteredGuestTotal.value = 0
    loadError.value = loadErrorMessage(error)
    toast.error(loadError.value)
  } finally { guestLoading.value = false }
}

async function load() {
  loading.value = true
  selectAllRecipients()
  templateParams.value = {}
  guestSearch.value = ''
  guestPage.value = 1
  loadError.value = ''
  createdCampaignId.value = ''
  createdCampaignName.value = ''
  try {
    const eventResponse = await rsvpService.get(props.eventId)
    const event = responsePayload(eventResponse)
    const [scheduleResult, templateResult, recipientResult] = await Promise.allSettled([
      rsvpService.listReminders(props.eventId),
      templatesService.list({ status: 'APPROVED', account: event.whatsapp_account, limit: 200 }),
      rsvpService.guests(props.eventId, { journey_status: 'not_started', page: 1, limit: guestLimit }),
    ])
    const errors: unknown[] = []

    if (scheduleResult.status === 'fulfilled') schedules.value = validSchedules(scheduleResult.value)
    else { schedules.value = []; errors.push(scheduleResult.reason) }

    if (templateResult.status === 'fulfilled') templates.value = validTemplates(templateResult.value)
    else { templates.value = []; errors.push(templateResult.reason) }

    if (recipientResult.status === 'fulfilled') applyRecipientResponse(recipientResult.value)
    else { guests.value = []; filteredGuestTotal.value = 0; recipientTotal.value = 0; errors.push(recipientResult.reason) }

    templateId.value = event.reminder_template_id || ''
    syncTemplateParams()
    if (errors.length) {
      loadError.value = loadErrorMessage(errors[0])
      toast.error(loadError.value)
    }
  } catch (error) {
    schedules.value = []
    templates.value = []
    guests.value = []
    filteredGuestTotal.value = 0
    recipientTotal.value = 0
    loadError.value = loadErrorMessage(error)
    toast.error(loadError.value)
  } finally { loading.value = false }
}

watch(() => props.open, value => {
  if (value) load()
}, { flush: 'sync' })
watch(templateParamNames, syncTemplateParams)

function close() { emit('update:open', false) }
function closeOnEscape(event: KeyboardEvent) {
  if (props.open && event.key === 'Escape') close()
}
onMounted(() => window.addEventListener('keydown', closeOnEscape))
onUnmounted(() => window.removeEventListener('keydown', closeOnEscape))

function onRecipientSearch() {
  if (searchTimer) window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(() => { guestPage.value = 1; loadRecipients() }, 300)
}
function setGuestPage(page: number) { guestPage.value = page; loadRecipients() }
async function send(all: boolean) {
  if (!templateId.value) { toast.error(t('rsvp.selectReminderTemplate')); return }
  if (missingTemplateParams.value.length) { toast.error(t('rsvp.reminderVariablesRequired')); return }
  const responseIds = props.selectedIds.filter(isRecipientSelected)
  if ((!all && !responseIds.length) || (all && !includedCount.value)) return
  loading.value = true
  try {
    const response = await rsvpService.sendReminders(props.eventId, all
      ? allRecipientsSelected.value
        ? { all_not_started: true, exclude_response_ids: [...excludedIds.value], template_id: templateId.value, template_params: templateParams.value }
        : { response_ids: [...includedIds.value], template_id: templateId.value, template_params: templateParams.value }
      : { response_ids: responseIds, template_id: templateId.value, template_params: templateParams.value })
    const data = (response.data as any).data || response.data
    if (data.campaign_id) {
      createdCampaignId.value = data.campaign_id
      createdCampaignName.value = data.campaign_name || ''
      toast.success(t('rsvp.reminderCampaignCreated', { queued: data.queued || 0, skipped: data.skipped || 0 }))
    } else {
      toast.success(t('rsvp.noEligibleReminderRecipients'))
    }
    emit('changed')
  } catch (error: any) { toast.error(error?.response?.data?.message || t('rsvp.reminderFailed')) }
  finally { loading.value = false }
}
async function schedule() {
  if (!scheduledAt.value || !templateId.value) return
  if (missingTemplateParams.value.length) { toast.error(t('rsvp.reminderVariablesRequired')); return }
  loading.value = true
  try {
    await rsvpService.createReminder(props.eventId, { scheduled_at: new Date(scheduledAt.value).toISOString(), template_id: templateId.value, template_params: templateParams.value })
    scheduledAt.value = ''; toast.success(t('rsvp.reminderScheduled')); await load()
  } catch (error: any) { toast.error(error?.response?.data?.message || t('rsvp.reminderFailed')) }
  finally { loading.value = false }
}
async function cancel(item: Schedule) { await rsvpService.cancelReminder(props.eventId, item.id); await load() }
async function viewCampaigns() {
  close()
  await router.push('/campaigns')
}
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4" @click.self="close">
      <section role="dialog" aria-modal="true" aria-labelledby="rsvp-reminder-dialog-title" class="relative grid w-full max-w-2xl max-h-[90vh] gap-4 overflow-y-auto rounded-lg border bg-background p-6 shadow-lg">
        <button type="button" class="absolute right-4 top-4 rounded-sm opacity-70 hover:opacity-100 focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2" :aria-label="t('common.close')" @click="close">
          <X class="h-4 w-4" />
        </button>
        <header class="flex flex-col space-y-1.5 text-center sm:text-left">
          <h2 id="rsvp-reminder-dialog-title" class="text-lg font-semibold leading-none tracking-tight">{{ t('rsvp.remindersTitle') }}</h2>
          <p class="text-sm text-muted-foreground">{{ t('rsvp.remindersHint') }}</p>
        </header>
      <div v-if="loadError" role="alert" class="flex items-center justify-between gap-3 rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
        <span>{{ loadError }}</span><Button variant="outline" size="sm" :disabled="loading" @click="load">{{ t('common.retry') }}</Button>
      </div>
      <div v-if="loading" class="flex justify-center p-2"><Loader2 class="h-5 w-5 animate-spin" /></div>
      <div class="space-y-5">
        <label class="block text-sm"><span>{{ t('rsvp.reminderTemplate') }}</span><select v-model="templateId" class="mt-1 w-full rounded border bg-transparent px-2 py-2"><option value="">{{ t('rsvp.selectTemplate') }}</option><option v-for="tpl in templates" :key="tpl.id" :value="tpl.id">{{ tpl.name }}</option></select></label>
        <div v-if="templateParamNames.length" class="space-y-3 rounded-lg border p-4">
          <div><div class="font-medium">{{ t('rsvp.reminderVariableMappings') }}</div><p class="text-xs text-muted-foreground">{{ t('rsvp.reminderVariableHint') }}</p></div>
          <label v-for="name in templateParamNames" :key="name" class="grid gap-1 text-sm sm:grid-cols-[120px_1fr] sm:items-center">
            <span class="font-mono">{{ formatTemplateParam(name) }}</span>
            <Input v-model="templateParams[name]" list="rsvp-reminder-variable-values" :placeholder="t('rsvp.reminderVariablePlaceholder', { token: formatTemplateParam('member_name') })" />
          </label>
          <datalist id="rsvp-reminder-variable-values">
            <option value="{{member_name}}">Member name</option><option value="{{member_phone}}">Member phone</option>
            <option value="{{event_name}}">Event name</option><option value="{{event_date}}">Event date</option>
            <option value="{{event_description}}">Event description</option><option value="{{event_keyword}}">Event keyword</option>
          </datalist>
        </div>

        <div class="space-y-3 rounded-lg border p-4">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div class="font-medium">{{ t('rsvp.repromptRecipients') }}</div>
            <div class="text-sm text-muted-foreground">{{ t('rsvp.recipientSelectionSummary', { selected: includedCount, total: recipientTotal }) }}</div>
          </div>
          <div class="flex items-center gap-2">
            <Button variant="outline" size="sm" :disabled="allRecipientsSelected && !excludedIds.size" @click="selectAllRecipients">{{ t('common.selectAll') }}</Button>
            <Button variant="outline" size="sm" :disabled="!includedCount" @click="clearAllRecipients">{{ t('rsvp.clearAllRecipients') }}</Button>
          </div>
          <div class="relative">
            <Search class="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input v-model="guestSearch" class="pl-9" :placeholder="t('rsvp.searchResponses')" @input="onRecipientSearch" />
          </div>
          <div v-if="guestLoading" class="flex justify-center py-5"><Loader2 class="h-5 w-5 animate-spin" /></div>
          <div v-else-if="!guests.length" class="py-4 text-center text-sm text-muted-foreground">{{ t('rsvp.noResponses') }}</div>
          <div v-else class="max-h-56 divide-y overflow-y-auto rounded-md border">
            <label v-for="guest in guests" :key="guest.id" class="flex cursor-pointer items-center gap-3 px-3 py-2.5 hover:bg-muted/40">
              <input type="checkbox" :checked="isRecipientSelected(guest.id)" @change="toggleRecipient(guest.id)" />
              <span class="min-w-0 flex-1"><span class="block truncate text-sm font-medium">{{ guest.contact?.profile_name || guest.phone_number }}</span><span class="block text-xs text-muted-foreground">{{ guest.phone_number }}</span></span>
              <span v-if="!isRecipientSelected(guest.id)" class="text-xs text-muted-foreground">{{ t('common.remove') }}</span>
            </label>
          </div>
          <div v-if="guestPages > 1" class="flex items-center justify-between text-sm">
            <Button variant="outline" size="sm" :disabled="guestPage <= 1" @click="setGuestPage(guestPage - 1)">‹</Button>
            <span class="text-muted-foreground">{{ guestPage }} / {{ guestPages }}</span>
            <Button variant="outline" size="sm" :disabled="guestPage >= guestPages" @click="setGuestPage(guestPage + 1)">›</Button>
          </div>
        </div>

        <div class="grid gap-2 sm:grid-cols-2"><Button :disabled="loading || !selectedSendCount" @click="send(false)"><Send class="mr-2 h-4 w-4" />{{ t('rsvp.remindSelected', { count: selectedSendCount }) }}</Button><Button variant="outline" :disabled="loading || !includedCount" @click="send(true)">{{ allRecipientsSelected && !excludedIds.size ? `${t('rsvp.remindAllNotStarted')} (${includedCount})` : t('rsvp.remindSelected', { count: includedCount }) }}</Button></div>
        <div v-if="createdCampaignId" class="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-primary/30 bg-primary/5 p-3 text-sm">
          <div><div class="font-medium">{{ t('rsvp.reminderCampaignReady') }}</div><div class="text-muted-foreground">{{ createdCampaignName }}</div></div>
          <Button variant="outline" size="sm" @click="viewCampaigns"><ExternalLink class="mr-2 h-4 w-4" />{{ t('rsvp.viewCampaigns') }}</Button>
        </div>
        <div class="rounded-lg border p-4 space-y-3"><div class="font-medium">{{ t('rsvp.scheduleReminder') }}</div><div class="flex gap-2"><Input v-model="scheduledAt" type="datetime-local" /><Button :disabled="!scheduledAt || !templateId || loading" @click="schedule">{{ t('rsvp.schedule') }}</Button></div></div>
        <div class="space-y-2"><div class="font-medium">{{ t('rsvp.scheduledReminders') }}</div><p v-if="!schedules.length" class="text-sm text-muted-foreground">{{ t('rsvp.noScheduledReminders') }}</p><div v-for="item in schedules" :key="item.id" class="flex items-center justify-between rounded-lg border p-3 text-sm"><div><div>{{ formatScheduleDate(item.scheduled_at) }}</div><div class="text-xs text-muted-foreground">{{ item.status }} · {{ item.sent_count }} sent · {{ item.failed_count }} failed</div></div><Button v-if="item.status === 'pending'" variant="ghost" size="icon" @click="cancel(item)"><Trash2 class="h-4 w-4" /></Button></div></div>
      </div>
        <footer class="flex flex-col-reverse sm:flex-row sm:justify-end sm:space-x-2"><Button variant="outline" @click="close">{{ t('common.close') }}</Button></footer>
      </section>
    </div>
  </Teleport>
</template>
