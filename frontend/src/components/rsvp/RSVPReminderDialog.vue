<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { rsvpService, templatesService } from '@/services/api'
import { formatDateTimeIST } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Loader2, Search, Send, Trash2 } from 'lucide-vue-next'

interface Schedule { id: string; scheduled_at: string; template_id: string; status: string; sent_count: number; failed_count: number }
interface Guest { id: string; phone_number: string; contact?: { profile_name?: string } }

const props = defineProps<{ open: boolean; eventId: string; selectedIds: string[] }>()
const emit = defineEmits<{ 'update:open': [value: boolean]; changed: [] }>()
const { t } = useI18n()
const schedules = ref<Schedule[]>([])
const templates = ref<any[]>([])
const templateId = ref('')
const templateParams = ref<Record<string, string>>({})
const scheduledAt = ref('')
const loading = ref(false)
const guestLoading = ref(false)
const guests = ref<Guest[]>([])
const guestSearch = ref('')
const guestPage = ref(1)
const guestLimit = 10
const filteredGuestTotal = ref(0)
const recipientTotal = ref(0)
const excludedIds = ref<Set<string>>(new Set())
let searchTimer: number | undefined

const guestPages = computed(() => Math.max(1, Math.ceil(filteredGuestTotal.value / guestLimit)))
const includedCount = computed(() => Math.max(0, recipientTotal.value - excludedIds.value.size))
const selectedSendCount = computed(() => props.selectedIds.filter(id => !excludedIds.value.has(id)).length)
const selectedTemplate = computed(() => templates.value.find(template => template.id === templateId.value))
const templateParamNames = computed(() => {
  const template = selectedTemplate.value
  if (!template) return []
  const contents = [template.body_content || '']
  for (const button of template.buttons || []) {
    if (String(button?.type || '').toUpperCase() === 'URL') contents.push(button.url || '')
  }
  const matches = contents.join('\n').match(/\{\{([^}]+)\}\}/g) || []
  return [...new Set(matches.map(value => value.replace(/\{\{|\}\}/g, '').trim()).filter(Boolean))]
})
const missingTemplateParams = computed(() => templateParamNames.value.filter(name => !String(templateParams.value[name] || '').trim()))

function syncTemplateParams() {
  const next: Record<string, string> = {}
  for (const name of templateParamNames.value) next[name] = templateParams.value[name] || ''
  templateParams.value = next
}

function unwrap(response: any, key: string) { const data = response?.data?.data || response?.data || {}; return data[key] || [] }
function formatTemplateParam(name: string) { return '{' + '{' + name + '}' + '}' }

async function loadRecipients() {
  guestLoading.value = true
  try {
    const response = await rsvpService.guests(props.eventId, {
      journey_status: 'not_started', search: guestSearch.value || undefined,
      page: guestPage.value, limit: guestLimit,
    })
    const data = (response.data as any).data || response.data
    guests.value = data.guests || []
    filteredGuestTotal.value = data.total || 0
    if (!guestSearch.value) recipientTotal.value = data.total || 0
  } finally { guestLoading.value = false }
}

async function load() {
  loading.value = true
  excludedIds.value = new Set()
  templateParams.value = {}
  guestSearch.value = ''
  guestPage.value = 1
  try {
    const eventResponse = await rsvpService.get(props.eventId)
    const event = (eventResponse.data as any).data || eventResponse.data
    const [scheduleResponse, templateResponse] = await Promise.all([
      rsvpService.listReminders(props.eventId),
      templatesService.list({ status: 'APPROVED', account: event.whatsapp_account, limit: 200 }),
      loadRecipients(),
    ])
    schedules.value = unwrap(scheduleResponse, 'reminders')
    templates.value = unwrap(templateResponse, 'templates')
    templateId.value = event.reminder_template_id || ''
    syncTemplateParams()
  } finally { loading.value = false }
}

watch(() => props.open, value => { if (value) load() })
watch(templateParamNames, syncTemplateParams)

function onRecipientSearch() {
  if (searchTimer) window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(() => { guestPage.value = 1; loadRecipients() }, 300)
}
function setGuestPage(page: number) { guestPage.value = page; loadRecipients() }
function toggleRecipient(id: string) {
  const next = new Set(excludedIds.value)
  if (next.has(id)) next.delete(id); else next.add(id)
  excludedIds.value = next
}

async function send(all: boolean) {
  if (!templateId.value) { toast.error(t('rsvp.selectReminderTemplate')); return }
  if (missingTemplateParams.value.length) { toast.error(t('rsvp.reminderVariablesRequired')); return }
  const responseIds = props.selectedIds.filter(id => !excludedIds.value.has(id))
  if ((!all && !responseIds.length) || (all && !includedCount.value)) return
  loading.value = true
  try {
    const response = await rsvpService.sendReminders(props.eventId, all
      ? { all_not_started: true, exclude_response_ids: [...excludedIds.value], template_id: templateId.value, template_params: templateParams.value }
      : { response_ids: responseIds, template_id: templateId.value, template_params: templateParams.value })
    const data = (response.data as any).data || response.data
    toast.success(t('rsvp.reminderResult', { sent: data.sent || 0, skipped: data.skipped || 0, failed: data.failed || 0 }))
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
</script>

<template>
  <Dialog :open="open" @update:open="emit('update:open', $event)">
    <DialogContent class="max-w-2xl max-h-[90vh] overflow-y-auto">
      <DialogHeader><DialogTitle>{{ t('rsvp.remindersTitle') }}</DialogTitle><DialogDescription>{{ t('rsvp.remindersHint') }}</DialogDescription></DialogHeader>
      <div v-if="loading" class="flex justify-center p-2"><Loader2 class="h-5 w-5 animate-spin" /></div>
      <div class="space-y-5">
        <label class="block text-sm"><span>{{ t('rsvp.reminderTemplate') }}</span><select v-model="templateId" class="mt-1 w-full rounded border bg-transparent px-2 py-2"><option value="">{{ t('rsvp.selectTemplate') }}</option><option v-for="tpl in templates" :key="tpl.id" :value="tpl.id">{{ tpl.name }}</option></select></label>
        <div v-if="templateParamNames.length" class="space-y-3 rounded-lg border p-4">
          <div><div class="font-medium">{{ t('rsvp.reminderVariableMappings') }}</div><p class="text-xs text-muted-foreground">{{ t('rsvp.reminderVariableHint') }}</p></div>
          <label v-for="name in templateParamNames" :key="name" class="grid gap-1 text-sm sm:grid-cols-[120px_1fr] sm:items-center">
            <span class="font-mono">{{ formatTemplateParam(name) }}</span>
            <Input v-model="templateParams[name]" list="rsvp-reminder-variable-values" :placeholder="t('rsvp.reminderVariablePlaceholder')" />
          </label>
          <datalist id="rsvp-reminder-variable-values">
            <option value="{{member_name}}">Member name</option><option value="{{member_phone}}">Member phone</option>
            <option value="{{event_name}}">Event name</option><option value="{{event_date}}">Event date</option>
            <option value="{{event_description}}">Event description</option><option value="{{event_keyword}}">Event keyword</option>
          </datalist>
        </div>

        <div class="space-y-3 rounded-lg border p-4">
          <div class="flex items-center justify-between gap-3">
            <div class="font-medium">{{ t('rsvp.repromptRecipients') }}</div>
            <div class="text-sm text-muted-foreground">{{ t('rsvp.selectedGuests', { count: includedCount }) }}</div>
          </div>
          <div class="relative">
            <Search class="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input v-model="guestSearch" class="pl-9" :placeholder="t('rsvp.searchResponses')" @input="onRecipientSearch" />
          </div>
          <div v-if="guestLoading" class="flex justify-center py-5"><Loader2 class="h-5 w-5 animate-spin" /></div>
          <div v-else-if="!guests.length" class="py-4 text-center text-sm text-muted-foreground">{{ t('rsvp.noResponses') }}</div>
          <div v-else class="max-h-56 divide-y overflow-y-auto rounded-md border">
            <label v-for="guest in guests" :key="guest.id" class="flex cursor-pointer items-center gap-3 px-3 py-2.5 hover:bg-muted/40">
              <input type="checkbox" :checked="!excludedIds.has(guest.id)" @change="toggleRecipient(guest.id)" />
              <span class="min-w-0 flex-1"><span class="block truncate text-sm font-medium">{{ guest.contact?.profile_name || guest.phone_number }}</span><span class="block text-xs text-muted-foreground">{{ guest.phone_number }}</span></span>
              <span v-if="excludedIds.has(guest.id)" class="text-xs text-muted-foreground">{{ t('common.remove') }}</span>
            </label>
          </div>
          <div v-if="guestPages > 1" class="flex items-center justify-between text-sm">
            <Button variant="outline" size="sm" :disabled="guestPage <= 1" @click="setGuestPage(guestPage - 1)">‹</Button>
            <span class="text-muted-foreground">{{ guestPage }} / {{ guestPages }}</span>
            <Button variant="outline" size="sm" :disabled="guestPage >= guestPages" @click="setGuestPage(guestPage + 1)">›</Button>
          </div>
        </div>

        <div class="grid gap-2 sm:grid-cols-2"><Button :disabled="loading || !selectedSendCount" @click="send(false)"><Send class="mr-2 h-4 w-4" />{{ t('rsvp.remindSelected', { count: selectedSendCount }) }}</Button><Button variant="outline" :disabled="loading || !includedCount" @click="send(true)">{{ t('rsvp.remindAllNotStarted') }} ({{ includedCount }})</Button></div>
        <div class="rounded-lg border p-4 space-y-3"><div class="font-medium">{{ t('rsvp.scheduleReminder') }}</div><div class="flex gap-2"><Input v-model="scheduledAt" type="datetime-local" /><Button :disabled="!scheduledAt || !templateId || loading" @click="schedule">{{ t('rsvp.schedule') }}</Button></div></div>
        <div class="space-y-2"><div class="font-medium">{{ t('rsvp.scheduledReminders') }}</div><p v-if="!schedules.length" class="text-sm text-muted-foreground">{{ t('rsvp.noScheduledReminders') }}</p><div v-for="item in schedules" :key="item.id" class="flex items-center justify-between rounded-lg border p-3 text-sm"><div><div>{{ formatDateTimeIST(item.scheduled_at) }}</div><div class="text-xs text-muted-foreground">{{ item.status }} · {{ item.sent_count }} sent · {{ item.failed_count }} failed</div></div><Button v-if="item.status === 'pending'" variant="ghost" size="icon" @click="cancel(item)"><Trash2 class="h-4 w-4" /></Button></div></div>
      </div>
      <DialogFooter><Button variant="outline" @click="emit('update:open', false)">{{ t('common.close') }}</Button></DialogFooter>
    </DialogContent>
  </Dialog>
</template>
