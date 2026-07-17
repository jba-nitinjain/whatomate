<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { rsvpService, templatesService, chatbotService } from '@/services/api'
import { getErrorMessage } from '@/lib/api-utils'
import { responseCollection, responsePayload, templateParameterNames } from './reminder-dialog-utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ExternalLink, Loader2, Send, X } from 'lucide-vue-next'

interface Flow { id: string; name: string; whatsapp_account?: string }
interface SkippedGuest { response_id: string; name: string; phone: string; reason: string }
interface StagedFollowUpMedia { staging_id: string; filename: string; mime_type: string }

// Header types that require an attached file before the template can send at
// all (mirrors RSVPReminderDialog.vue - Meta rejects otherwise, error 132012).
const MEDIA_HEADER_TYPES = ['IMAGE', 'VIDEO', 'DOCUMENT']

const AUDIENCES = ['missing_answer', 'not_started', 'responded_yes', 'responded_no'] as const
type Audience = typeof AUDIENCES[number]

const props = defineProps<{ open: boolean; eventId: string }>()
const emit = defineEmits<{ 'update:open': [value: boolean]; changed: [] }>()
const { t } = useI18n()
const router = useRouter()

const loading = ref(false)
const loadError = ref('')
const eventWhatsAppAccount = ref('')
const primaryFlowId = ref('')

const audience = ref<Audience>('missing_answer')
const answerKey = ref('')

const templates = ref<any[]>([])
const templateId = ref('')
const templateParams = ref<Record<string, string>>({})
const stagedMedia = ref<StagedFollowUpMedia | null>(null)
const mediaUploading = ref(false)
const mediaUploadError = ref('')

const flows = ref<Flow[]>([])
const flowId = ref('')

const previewLoading = ref(false)
const previewError = ref('')
const eligibleCount = ref(0)
const skippedGuests = ref<SkippedGuest[]>([])

const sending = ref(false)
const createdCampaignId = ref('')
const createdCampaignName = ref('')

let previewTimer: number | undefined
let previewRequestId = 0

const selectedTemplate = computed(() => templates.value.find(template => template.id === templateId.value))
const templateParamNames = computed(() => templateParameterNames(selectedTemplate.value))
const missingTemplateParams = computed(() => templateParamNames.value.filter(name => !String(templateParams.value[name] || '').trim()))
const selectedTemplateHeaderType = computed(() => String(selectedTemplate.value?.header_type || '').toUpperCase())
const templateNeedsMedia = computed(() => MEDIA_HEADER_TYPES.includes(selectedTemplateHeaderType.value))
const mediaMissing = computed(() => templateNeedsMedia.value && !stagedMedia.value)
const mediaAccept = computed(() => {
  switch (selectedTemplateHeaderType.value) {
    case 'IMAGE': return 'image/jpeg,image/png,image/webp'
    case 'VIDEO': return 'video/mp4,video/3gpp'
    case 'DOCUMENT': return '.pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx'
    default: return '*/*'
  }
})
const mediaMissingReason = computed(() => mediaMissing.value
  ? t('rsvp.reminderMediaRequired', { template: selectedTemplate.value?.name || '', type: selectedTemplateHeaderType.value.toLowerCase() })
  : '')

// Everything that must be true before Send is allowed, in the order a user
// would fix them working top-to-bottom through the three pickers (who, what
// to send, what to ask).
const blockReason = computed(() => {
  if (audience.value === 'missing_answer' && !answerKey.value.trim()) return t('rsvp.followUpAnswerKeyRequired')
  if (!templateId.value) return t('rsvp.followUpTemplateRequired')
  if (missingTemplateParams.value.length) return t('rsvp.followUpVariablesRequired')
  if (mediaMissing.value) return mediaMissingReason.value
  if (!flowId.value) return t('rsvp.followUpFlowRequired')
  return ''
})
const sendDisabled = computed(() => loading.value || sending.value || !!blockReason.value)

function syncTemplateParams() {
  const next: Record<string, string> = {}
  for (const name of templateParamNames.value) next[name] = templateParams.value[name] || ''
  templateParams.value = next
}
function formatTemplateParam(name: string) { return '{' + '{' + name + '}' + '}' }
function isRecord(value: unknown): value is Record<string, any> { return !!value && typeof value === 'object' && !Array.isArray(value) }
function validTemplates(response: any) { return responseCollection<any>(response, 'templates').filter(isRecord) }
function isFlow(value: unknown): value is Flow { return isRecord(value) && typeof value.id === 'string' && typeof value.name === 'string' }
function isSkippedGuest(value: unknown): value is SkippedGuest { return isRecord(value) && typeof value.response_id === 'string' && typeof value.reason === 'string' }
function validSkippedGuests(response: any) { return responseCollection<SkippedGuest>(response, 'skipped').filter(isSkippedGuest) }

function loadErrorMessage(error?: unknown) {
  const fallback = t('rsvp.followUpLoadFailed')
  return error ? getErrorMessage(error, fallback) : fallback
}

// Who a follow-up flow can be: not the event's own primary RSVP flow (the
// server rejects that - re-asking attendance via the merge path would be a
// confusing half-update), and scoped to the event's WhatsApp account or
// unscoped (an org-level default flow, never account-restricted). Mirrors
// rsvpFollowUpFlowIsEventPrimary / rsvpFollowUpFlowWrongAccount server-side.
function eligibleFollowUpFlows(all: Flow[], primary: string, account: string): Flow[] {
  return all.filter(flow => flow.id !== primary && (!flow.whatsapp_account || flow.whatsapp_account === account))
}

async function loadPreview() {
  if (audience.value === 'missing_answer' && !answerKey.value.trim()) {
    eligibleCount.value = 0
    skippedGuests.value = []
    previewError.value = ''
    return
  }
  const requestId = ++previewRequestId
  previewLoading.value = true
  previewError.value = ''
  try {
    const response = await rsvpService.followUpPreview(props.eventId, audience.value, audience.value === 'missing_answer' ? answerKey.value.trim() : undefined)
    if (requestId !== previewRequestId) return
    const data = responsePayload(response)
    eligibleCount.value = typeof data.eligible === 'number' ? data.eligible : 0
    skippedGuests.value = validSkippedGuests(response)
  } catch (error) {
    if (requestId !== previewRequestId) return
    eligibleCount.value = 0
    skippedGuests.value = []
    previewError.value = getErrorMessage(error, t('rsvp.followUpCountFailed'))
  } finally {
    if (requestId === previewRequestId) previewLoading.value = false
  }
}

function onAudienceChange() { loadPreview() }
function onAnswerKeyInput() {
  if (previewTimer) window.clearTimeout(previewTimer)
  previewTimer = window.setTimeout(loadPreview, 300)
}

async function load() {
  loading.value = true
  audience.value = 'missing_answer'
  answerKey.value = ''
  templateId.value = ''
  templateParams.value = {}
  flowId.value = ''
  stagedMedia.value = null
  mediaUploadError.value = ''
  loadError.value = ''
  createdCampaignId.value = ''
  createdCampaignName.value = ''
  eligibleCount.value = 0
  skippedGuests.value = []
  try {
    const eventResponse = await rsvpService.get(props.eventId)
    const event = responsePayload(eventResponse)
    eventWhatsAppAccount.value = event.whatsapp_account || ''
    primaryFlowId.value = event.flow_id || ''

    const [templateResult, flowResult] = await Promise.allSettled([
      templatesService.list({ status: 'APPROVED', account: event.whatsapp_account, limit: 200 }),
      chatbotService.listFlows({ limit: 200 }),
    ])
    const errors: unknown[] = []

    if (templateResult.status === 'fulfilled') templates.value = validTemplates(templateResult.value)
    else { templates.value = []; errors.push(templateResult.reason) }

    if (flowResult.status === 'fulfilled') {
      const allFlows = responseCollection<Flow>(flowResult.value, 'flows').filter(isFlow)
      flows.value = eligibleFollowUpFlows(allFlows, primaryFlowId.value, eventWhatsAppAccount.value)
    } else { flows.value = []; errors.push(flowResult.reason) }

    if (errors.length) {
      loadError.value = loadErrorMessage(errors[0])
      toast.error(loadError.value)
    }
    await loadPreview()
  } catch (error) {
    templates.value = []
    flows.value = []
    loadError.value = loadErrorMessage(error)
    toast.error(loadError.value)
  } finally { loading.value = false }
}

// Loads (and resets) dialog state whenever it opens - same pattern as
// RSVPReminderDialog.vue. load() reassigns audience/templateId/etc directly
// rather than going through onAudienceChange/onTemplateChange, so this does
// not double-trigger loadPreview; load() calls it once itself at the end.
watch(() => props.open, value => { if (value) load() }, { flush: 'sync' })

function close() { emit('update:open', false) }
function closeOnEscape(event: KeyboardEvent) {
  if (props.open && event.key === 'Escape') close()
}
onMounted(() => window.addEventListener('keydown', closeOnEscape))
onUnmounted(() => window.removeEventListener('keydown', closeOnEscape))

async function onMediaFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  // Same race guard as RSVPReminderDialog.vue: capture which template this
  // upload is for, and discard the response if the user switched templates
  // (which clears stagedMedia via the templateId watcher below) before the
  // upload resolved - otherwise a slow upload could silently reattach a file
  // staged for a template the user is no longer on.
  const requestTemplateId = templateId.value
  mediaUploading.value = true
  mediaUploadError.value = ''
  try {
    const response = await rsvpService.uploadReminderMedia(props.eventId, file)
    if (templateId.value !== requestTemplateId) return
    const data = responsePayload(response)
    if (typeof data.staging_id === 'string' && data.staging_id) {
      stagedMedia.value = {
        staging_id: data.staging_id,
        filename: typeof data.filename === 'string' && data.filename ? data.filename : file.name,
        mime_type: typeof data.mime_type === 'string' && data.mime_type ? data.mime_type : file.type,
      }
    }
  } catch (error) {
    if (templateId.value !== requestTemplateId) return
    stagedMedia.value = null
    mediaUploadError.value = getErrorMessage(error, t('rsvp.reminderMediaUploadFailed'))
    toast.error(mediaUploadError.value)
  } finally {
    mediaUploading.value = false
    input.value = ''
  }
}
function clearStagedMedia() { stagedMedia.value = null }
function onTemplateChange() {
  // A file staged for one template's header type is not necessarily valid
  // for another, so switching templates clears whatever was attached.
  stagedMedia.value = null
  mediaUploadError.value = ''
  syncTemplateParams()
}

// Best-effort extraction of the first recipient failure reason from a send
// response, mirroring RSVPReminderDialog.vue's firstReminderErrorMessage.
// The follow-up send endpoint does not currently return sent/failed at all
// (only requested/queued/skipped), so this branch is defensive and currently
// unreachable - kept identical to the reminder dialog rather than silently
// dropped, so a future response shape change is handled the same way.
function firstFollowUpErrorMessage(data: Record<string, any>): string {
  const candidates = [data.recipients, data.errors].find(Array.isArray) || []
  for (const item of candidates) {
    if (isRecord(item) && typeof item.error_message === 'string' && item.error_message.trim()) return item.error_message.trim()
  }
  return typeof data.error_message === 'string' ? data.error_message.trim() : ''
}

async function send() {
  if (blockReason.value) { toast.error(blockReason.value); return }
  sending.value = true
  try {
    const media = stagedMedia.value
      ? { staging_id: stagedMedia.value.staging_id, staging_filename: stagedMedia.value.filename }
      : {}
    const response = await rsvpService.sendFollowUp(props.eventId, {
      audience: audience.value,
      answer_key: audience.value === 'missing_answer' ? answerKey.value.trim() : undefined,
      flow_id: flowId.value,
      template_id: templateId.value,
      template_params: templateParams.value,
      ...media,
    })
    const data = responsePayload(response)
    const sentCount = Number(data.sent) || 0
    const failedCount = Number(data.failed) || 0
    const skippedCount = Array.isArray(data.skipped) ? data.skipped.length : Number(data.skipped) || 0
    if (sentCount === 0 && failedCount > 0) {
      // A run where every recipient failed must not read as success - same
      // rule as RSVPReminderDialog.vue, for the same reason.
      const summary = t('rsvp.followUpResult', { queued: Number(data.queued) || 0, skipped: skippedCount, failed: failedCount })
      const detail = firstFollowUpErrorMessage(data)
      toast.error(detail ? `${summary} ${detail}` : summary)
    } else if (data.campaign_id) {
      createdCampaignId.value = data.campaign_id
      createdCampaignName.value = data.campaign_name || ''
      toast.success(t('rsvp.followUpCampaignCreated', { queued: data.queued || 0, skipped: skippedCount }))
      emit('changed')
      // The audience shrinks as guests answer, so refresh the count rather
      // than leave the pre-send number showing after a successful send.
      await loadPreview()
    } else {
      toast.success(t('rsvp.noEligibleFollowUpRecipients'))
    }
  } catch (error) {
    toast.error(getErrorMessage(error, t('rsvp.followUpFailed')))
  } finally { sending.value = false }
}

async function viewCampaigns() {
  close()
  await router.push('/campaigns')
}
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4" @click.self="close">
      <section role="dialog" aria-modal="true" aria-labelledby="rsvp-followup-dialog-title" class="relative grid w-full max-w-2xl max-h-[90vh] gap-4 overflow-y-auto rounded-lg border bg-background p-6 shadow-lg">
        <button type="button" class="absolute right-4 top-4 rounded-sm opacity-70 hover:opacity-100 focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2" :aria-label="t('common.close')" @click="close">
          <X class="h-4 w-4" />
        </button>
        <header class="flex flex-col space-y-1.5 text-center sm:text-left">
          <h2 id="rsvp-followup-dialog-title" class="text-lg font-semibold leading-none tracking-tight">{{ t('rsvp.followUpTitle') }}</h2>
          <p class="text-sm text-muted-foreground">{{ t('rsvp.followUpHint') }}</p>
        </header>
        <div v-if="loadError" role="alert" class="flex items-center justify-between gap-3 rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          <span>{{ loadError }}</span><Button variant="outline" size="sm" :disabled="loading" @click="load">{{ t('common.retry') }}</Button>
        </div>
        <div v-if="loading" class="flex justify-center p-2"><Loader2 class="h-5 w-5 animate-spin" /></div>
        <div class="space-y-5">
          <div class="space-y-3 rounded-lg border p-4">
            <div class="font-medium">{{ t('rsvp.followUpWho') }}</div>
            <select v-model="audience" class="mt-1 w-full rounded border bg-transparent px-2 py-2" @change="onAudienceChange">
              <option value="missing_answer">{{ t('rsvp.followUpAudienceMissingAnswer') }}</option>
              <option value="not_started">{{ t('rsvp.followUpAudienceNotStarted') }}</option>
              <option value="responded_yes">{{ t('rsvp.followUpAudienceRespondedYes') }}</option>
              <option value="responded_no">{{ t('rsvp.followUpAudienceRespondedNo') }}</option>
            </select>
            <div v-if="audience === 'missing_answer'" class="space-y-1">
              <label class="block text-sm"><span>{{ t('rsvp.followUpAnswerKey') }}</span>
                <Input v-model="answerKey" class="mt-1" :placeholder="t('rsvp.followUpAnswerKeyPlaceholder')" @input="onAnswerKeyInput" />
              </label>
              <p class="text-xs text-muted-foreground">{{ t('rsvp.followUpAnswerKeyHint') }}</p>
            </div>

            <div class="rounded-md bg-muted/40 p-3 text-sm">
              <div v-if="previewLoading" class="flex items-center gap-2 text-muted-foreground"><Loader2 class="h-4 w-4 animate-spin" />{{ t('common.loading') }}</div>
              <div v-else-if="previewError" class="text-destructive">{{ previewError }}</div>
              <div v-else class="font-medium">{{ t('rsvp.followUpCount', { count: eligibleCount }) }}</div>
              <p class="mt-1 text-xs text-muted-foreground">{{ t('rsvp.followUpCountShrinks') }}</p>
            </div>

            <div v-if="skippedGuests.length" class="rounded-lg border p-3 text-sm">
              <details>
                <summary class="cursor-pointer font-medium">{{ t('rsvp.followUpSkippedSummary', { count: skippedGuests.length }) }}</summary>
                <ul class="mt-2 space-y-1 text-xs text-muted-foreground">
                  <li v-for="guest in skippedGuests" :key="guest.response_id">
                    <span class="font-medium text-foreground">{{ guest.name || guest.phone }}</span><span v-if="guest.name && guest.phone"> · {{ guest.phone }}</span> — {{ guest.reason }}
                  </li>
                </ul>
              </details>
            </div>
          </div>

          <div class="space-y-3 rounded-lg border p-4">
            <div class="font-medium">{{ t('rsvp.followUpWhatToSend') }}</div>
            <select v-model="templateId" class="mt-1 w-full rounded border bg-transparent px-2 py-2" @change="onTemplateChange">
              <option value="">{{ t('rsvp.selectTemplate') }}</option>
              <option v-for="tpl in templates" :key="tpl.id" :value="tpl.id">{{ tpl.name }}</option>
            </select>
            <div v-if="templateParamNames.length" class="space-y-3 rounded-lg border p-4">
              <div><div class="font-medium">{{ t('rsvp.reminderVariableMappings') }}</div><p class="text-xs text-muted-foreground">{{ t('rsvp.reminderVariableHint') }}</p></div>
              <label v-for="name in templateParamNames" :key="name" class="grid gap-1 text-sm sm:grid-cols-[120px_1fr] sm:items-center">
                <span class="font-mono">{{ formatTemplateParam(name) }}</span>
                <Input v-model="templateParams[name]" list="rsvp-followup-variable-values" :placeholder="t('rsvp.reminderVariablePlaceholder', { token: formatTemplateParam('member_name') })" />
              </label>
              <datalist id="rsvp-followup-variable-values">
                <option value="{{member_name}}">Member name</option><option value="{{member_phone}}">Member phone</option>
                <option value="{{event_name}}">Event name</option><option value="{{event_date}}">Event date</option>
                <option value="{{event_description}}">Event description</option><option value="{{event_keyword}}">Event keyword</option>
              </datalist>
            </div>

            <div v-if="templateNeedsMedia" class="space-y-2 rounded-lg border p-4">
              <div class="font-medium">{{ t('rsvp.reminderMediaLabel', { type: selectedTemplateHeaderType.toLowerCase() }) }}</div>
              <input type="file" :accept="mediaAccept" class="block w-full text-sm" :disabled="mediaUploading" @change="onMediaFileChange" />
              <p v-if="mediaUploading" class="text-xs text-muted-foreground">{{ t('rsvp.reminderMediaUploading') }}</p>
              <div v-else-if="stagedMedia" class="flex items-center justify-between gap-2 text-xs text-muted-foreground">
                <span class="truncate">{{ stagedMedia.filename }}</span>
                <Button type="button" variant="ghost" size="sm" @click="clearStagedMedia">{{ t('common.remove') }}</Button>
              </div>
              <p v-if="mediaUploadError" class="text-sm text-destructive">{{ mediaUploadError }}</p>
            </div>
          </div>

          <div class="space-y-3 rounded-lg border p-4">
            <div class="font-medium">{{ t('rsvp.followUpWhatToAsk') }}</div>
            <select v-model="flowId" class="mt-1 w-full rounded border bg-transparent px-2 py-2">
              <option value="">{{ t('rsvp.selectFlow') }}</option>
              <option v-for="fl in flows" :key="fl.id" :value="fl.id">{{ fl.name }}</option>
            </select>
            <p class="text-xs text-muted-foreground">{{ t('rsvp.followUpFlowHint') }}</p>
            <p v-if="!loading && !flows.length" class="text-xs text-muted-foreground">{{ t('rsvp.followUpNoFlows') }}</p>
          </div>

          <div v-if="blockReason" class="rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">{{ blockReason }}</div>
          <Button class="w-full" :disabled="sendDisabled" @click="send">
            <Send class="mr-2 h-4 w-4" />{{ t('rsvp.followUpSend', { count: eligibleCount }) }}
          </Button>

          <div v-if="createdCampaignId" class="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-primary/30 bg-primary/5 p-3 text-sm">
            <div><div class="font-medium">{{ t('rsvp.followUpCampaignReady') }}</div><div class="text-muted-foreground">{{ createdCampaignName }}</div></div>
            <Button variant="outline" size="sm" @click="viewCampaigns"><ExternalLink class="mr-2 h-4 w-4" />{{ t('rsvp.viewCampaigns') }}</Button>
          </div>
        </div>
        <footer class="flex flex-col-reverse sm:flex-row sm:justify-end sm:space-x-2"><Button variant="outline" @click="close">{{ t('common.close') }}</Button></footer>
      </section>
    </div>
  </Teleport>
</template>
