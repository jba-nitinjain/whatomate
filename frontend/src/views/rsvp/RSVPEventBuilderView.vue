<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { PageHeader } from '@/components/shared'
import { toast } from 'vue-sonner'
import { getErrorMessage } from '@/lib/api-utils'
import { rsvpService, accountsService, chatbotService, templatesService } from '@/services/api'
import { CalendarCheck, RefreshCw, Sparkles, HelpCircle, Plus, X, ArrowUp, ArrowDown } from 'lucide-vue-next'
import {
  type HeadcountContributorRow,
  contributorsToRows,
  contributorRowsToPayload,
  legacyHeadcountContributorRows,
  nextContributorRowKey,
} from './headcountContributors'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const id = computed(() => route.params.id as string | undefined)

const form = ref<any>({
  name: '', description: '', keyword: '', event_date: '', rsvp_close_at: '',
  whatsapp_account: '', flow_id: '', template_id: '', reminder_enabled: false,
  reminder_at: '', reminder_template_id: '',
  spouse_mobile_field: '', duplicate_message: '', access_mode: 'guest_list',
  not_invited_message: 'Sorry, this RSVP is limited to invited guests.'
})
// A brand-new event has nothing configured yet, so this starts prefilled with
// the same member + spouse pair the tally handler falls back to - saving the
// form as-is is then a no-op, not a behaviour change. Kept as its own typed
// ref (rather than a field on the loosely-typed `form`) so row index/key
// handling stays type-checked.
const headcountContributors = ref<HeadcountContributorRow[]>(legacyHeadcountContributorRows())
const status = ref('draft')
const saving = ref(false)
const creatingFlow = ref(false)
const generatingForm = ref(false)
const showHelp = ref(false)

// Dropdown option sources
const accounts = ref<any[]>([])
const flows = ref<any[]>([])
const templates = ref<any[]>([])

function unwrap(res: any, key: string): any[] {
  const d = res?.data?.data || res?.data || {}
  return d[key] || []
}

async function loadAccounts() {
  try { accounts.value = unwrap(await accountsService.list(), 'accounts') } catch { accounts.value = [] }
}
async function loadFlows() {
  try { flows.value = unwrap(await chatbotService.listFlows({ limit: 200 }), 'flows') } catch { flows.value = [] }
}
async function loadTemplates() {
  try {
    templates.value = unwrap(await templatesService.list({
      status: 'APPROVED',
      account: form.value.whatsapp_account || undefined,
      limit: 200,
    }), 'templates')
  } catch { templates.value = [] }
}

async function load() {
  if (!id.value) return
  const res = await rsvpService.get(id.value)
  const e = (res.data as any).data || res.data
  form.value = {
    name: e.name || '', description: e.description || '', keyword: e.keyword || '',
    event_date: e.event_date ? String(e.event_date).substring(0, 10) : '',
    rsvp_close_at: e.rsvp_close_at ? String(e.rsvp_close_at).substring(0, 10) : '',
    whatsapp_account: e.whatsapp_account || '', flow_id: e.flow_id || '',
    template_id: e.template_id || '', reminder_enabled: !!e.reminder_enabled,
    reminder_at: e.reminder_at ? String(e.reminder_at).substring(0, 16) : '',
    reminder_template_id: e.reminder_template_id || '',
    spouse_mobile_field: e.spouse_mobile_field || '', duplicate_message: e.duplicate_message || '',
    access_mode: e.access_mode || 'open_keyword',
    not_invited_message: e.not_invited_message || 'Sorry, this RSVP is limited to invited guests.'
  }
  // Nothing configured on this (possibly pre-existing) event is presented as
  // the same legacy pair the tally already falls back to, so opening this
  // page and clicking Save changes nothing about how the event is tallied.
  headcountContributors.value = e.headcount_contributors && e.headcount_contributors.length
    ? contributorsToRows(e.headcount_contributors)
    : legacyHeadcountContributorRows()
  status.value = e.status || 'draft'
}

function addContributorRow() {
  headcountContributors.value.push({
    _key: nextContributorRowKey(),
    label: '', answer_key: '', mode: 'boolean', match_values_text: ''
  })
}
function removeContributorRow(index: number) {
  headcountContributors.value.splice(index, 1)
}
function moveContributorRow(index: number, delta: number) {
  const rows = headcountContributors.value
  const target = index + delta
  if (target < 0 || target >= rows.length) return
  const [row] = rows.splice(index, 1)
  rows.splice(target, 0, row)
}

onMounted(async () => {
  await load()
  await Promise.all([loadAccounts(), loadFlows(), loadTemplates()])
})

function payload() {
  const f = form.value
  return {
    name: f.name,
    description: f.description,
    keyword: f.keyword,
    whatsapp_account: f.whatsapp_account,
    event_date: f.event_date ? new Date(f.event_date).toISOString() : null,
    rsvp_close_at: f.rsvp_close_at ? new Date(f.rsvp_close_at).toISOString() : null,
    reminder_enabled: f.reminder_enabled,
    reminder_at: f.reminder_at ? new Date(f.reminder_at).toISOString() : null,
    flow_id: f.flow_id || null,
    template_id: f.template_id || null,
    reminder_template_id: f.reminder_template_id || null,
    spouse_mobile_field: f.spouse_mobile_field || '',
    duplicate_message: f.duplicate_message || '',
    access_mode: f.access_mode,
    not_invited_message: f.not_invited_message || '',
    headcount_contributors: contributorRowsToPayload(headcountContributors.value)
  }
}

async function generateForm() {
  if (!id.value) { toast.error(t('rsvp.saveEventFirst')); return }
  generatingForm.value = true
  try {
    await rsvpService.generateFlowForm(id.value)
    toast.success(t('rsvp.flowFormCreated'))
  } catch (e: any) {
    toast.error(getErrorMessage(e, t('rsvp.flowFormFailed')))
  } finally {
    generatingForm.value = false
  }
}

async function save() {
  saving.value = true
  try {
    if (id.value) await rsvpService.update(id.value, payload())
    else await rsvpService.create(payload())
    toast.success(t('rsvp.save'))
    router.push('/rsvp')
  } catch (e: any) {
    toast.error(getErrorMessage(e, t('rsvp.save')))
  } finally {
    saving.value = false
  }
}

async function activate() {
  if (!id.value) return
  try {
    await rsvpService.activate(id.value)
    toast.success(t('rsvp.activate'))
    await load()
  } catch (e: any) {
    toast.error(getErrorMessage(e, t('rsvp.activateFailed')))
  }
}

async function close() {
  if (!id.value) return
  await rsvpService.close(id.value)
  toast.success(t('rsvp.close'))
  await load()
}

// One-click starter: build a ready-made RSVP question flow and select it.
async function createStarterFlow() {
  if (!form.value.whatsapp_account) {
    toast.error(t('rsvp.pickAccountFirst'))
    return
  }
  creatingFlow.value = true
  try {
    const body = {
      name: `RSVP flow – ${form.value.name || 'event'}`.slice(0, 90),
      whatsapp_account: form.value.whatsapp_account,
      description: 'Auto-generated starter RSVP flow',
      trigger_keywords: form.value.keyword ? [form.value.keyword] : ['RSVP'],
      completion_message: 'Thank you! Your RSVP has been recorded.',
      enabled: true,
      steps: [
        {
          step_name: 'attendance',
          message: 'Will you be attending?',
          message_type: 'buttons',
          input_type: 'button',
          buttons: [
            { id: 'yes', title: 'Attending' },
            { id: 'no', title: 'Not Attending' }
          ],
          store_as: 'attendance',
          // Self attendance needs no mobile (we already have the sender's number).
          conditional_next: { yes: 'spouse_attendance', no: 'spouse_attendance', default: '' }
        },
        {
          step_name: 'spouse_attendance',
          message: 'Will your spouse be attending?',
          message_type: 'buttons',
          input_type: 'button',
          buttons: [
            { id: 'yes', title: 'Attending' },
            { id: 'no', title: 'Not Attending' }
          ],
          store_as: 'spouse_attendance',
          // Only collect the spouse mobile when the spouse is attending.
          conditional_next: { yes: 'spouse_mobile', no: '__complete__', default: '' }
        },
        {
          step_name: 'spouse_mobile',
          message: "Please share your spouse's mobile number.",
          message_type: 'text',
          input_type: 'phone',
          store_as: 'spouse_mobile',
          validation_regex: '^[6-9][0-9]{9}$',
          validation_error: 'Please enter a valid 10-digit Indian mobile number (starting with 6, 7, 8 or 9).',
          retry_on_invalid: true,
          max_retries: 5
        }
      ]
    }
    const res = await chatbotService.createFlow(body)
    const created = (res.data as any).data || res.data
    const newId = created?.id || created?.flow?.id || created?.flow_id
    await loadFlows()
    if (newId) form.value.flow_id = newId
    // Wire the duplicate check to the spouse mobile the starter flow collects.
    if (!form.value.spouse_mobile_field) form.value.spouse_mobile_field = 'spouse_mobile'
    toast.success(t('rsvp.starterFlowCreated'))
  } catch (e: any) {
    toast.error(getErrorMessage(e, t('rsvp.starterFlowFailed')))
  } finally {
    creatingFlow.value = false
  }
}

const inputClass = 'w-full border rounded px-2 py-1 bg-transparent text-sm'
const templateExampleBody = "You're invited to {{1}}! Tap below to RSVP."
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="id ? t('rsvp.editTitle') : t('rsvp.newTitle')" :icon="CalendarCheck" back-link="/rsvp" />

    <ScrollArea class="flex-1">
      <div class="p-6">
        <div class="max-w-2xl mx-auto space-y-4">
          <Card>
            <CardContent class="pt-6 space-y-4">
              <label class="block"><span class="text-sm">{{ t('rsvp.name') }}</span>
                <input v-model="form.name" :class="inputClass" /></label>
              <label class="block"><span class="text-sm">{{ t('rsvp.description') }}</span>
                <textarea v-model="form.description" :class="inputClass"></textarea></label>
              <label class="block"><span class="text-sm">{{ t('rsvp.keyword') }}</span>
                <input v-model="form.keyword" :class="inputClass" /></label>

              <div class="space-y-2">
                <span class="text-sm">{{ t('rsvp.accessMode') }}</span>
                <div class="grid gap-2 sm:grid-cols-2">
                  <button type="button" :class="['rounded-lg border p-3 text-left text-sm', form.access_mode === 'guest_list' ? 'border-primary bg-primary/5' : 'border-border']" @click="form.access_mode = 'guest_list'">
                    <span class="font-medium">{{ t('rsvp.guestListOnly') }}</span>
                    <span class="mt-1 block text-xs text-muted-foreground">{{ t('rsvp.guestListOnlyHint') }}</span>
                  </button>
                  <button type="button" :class="['rounded-lg border p-3 text-left text-sm', form.access_mode === 'open_keyword' ? 'border-primary bg-primary/5' : 'border-border']" @click="form.access_mode = 'open_keyword'">
                    <span class="font-medium">{{ t('rsvp.openKeyword') }}</span>
                    <span class="mt-1 block text-xs text-muted-foreground">{{ t('rsvp.openKeywordHint') }}</span>
                  </button>
                </div>
              </div>
              <label v-if="form.access_mode === 'guest_list'" class="block"><span class="text-sm">{{ t('rsvp.notInvitedMessage') }}</span>
                <textarea v-model="form.not_invited_message" :class="inputClass"></textarea></label>

              <label class="block"><span class="text-sm">{{ t('rsvp.spouseMobileField') }}</span>
                <input v-model="form.spouse_mobile_field" :class="inputClass" :placeholder="t('rsvp.spouseMobileFieldPlaceholder')" />
                <span class="text-xs text-muted-foreground">{{ t('rsvp.spouseMobileFieldHint') }}</span></label>
              <label class="block"><span class="text-sm">{{ t('rsvp.duplicateMessage') }}</span>
                <textarea v-model="form.duplicate_message" :class="inputClass" :placeholder="t('rsvp.duplicateMessagePlaceholder')"></textarea>
                <span class="text-xs text-muted-foreground">{{ t('rsvp.duplicateMessageHint') }}</span></label>

              <!-- Headcount contributors -->
              <div>
                <div class="flex items-center justify-between">
                  <span class="text-sm">{{ t('rsvp.headcountContributors') }}</span>
                  <button type="button" class="text-xs text-primary hover:underline" @click="addContributorRow">
                    <Plus class="inline h-3 w-3 mr-1" />{{ t('rsvp.headcountContributorAdd') }}
                  </button>
                </div>
                <p class="text-xs text-muted-foreground mt-1 mb-2">{{ t('rsvp.headcountContributorsHint') }}</p>

                <div v-if="!headcountContributors.length" class="text-xs text-muted-foreground italic">
                  {{ t('rsvp.headcountContributorEmpty') }}
                </div>

                <div v-else class="space-y-2">
                  <div class="hidden sm:grid grid-cols-12 gap-2 text-xs text-muted-foreground px-1">
                    <span class="col-span-3">{{ t('rsvp.headcountContributorLabel') }}</span>
                    <span class="col-span-3">{{ t('rsvp.headcountContributorAnswerKey') }}</span>
                    <span class="col-span-2">{{ t('rsvp.headcountContributorMode') }}</span>
                    <span class="col-span-3">{{ t('rsvp.headcountContributorMatchValues') }}</span>
                    <span class="col-span-1"></span>
                  </div>

                  <div v-for="(row, idx) in headcountContributors" :key="row._key"
                       class="grid grid-cols-12 gap-2 items-center rounded-lg border p-2">
                    <input v-model="row.label" :class="[inputClass, 'col-span-12 sm:col-span-3']"
                           :placeholder="t('rsvp.headcountContributorLabel')" />
                    <input v-model="row.answer_key" :disabled="row.mode === 'attendance'"
                           :class="[inputClass, 'col-span-12 sm:col-span-3', row.mode === 'attendance' ? 'opacity-50' : '']"
                           :placeholder="t('rsvp.headcountContributorAnswerKeyPlaceholder')" />
                    <select v-model="row.mode" :class="[inputClass, 'col-span-6 sm:col-span-2']">
                      <option value="boolean">{{ t('rsvp.headcountContributorModeBoolean') }}</option>
                      <option value="numeric">{{ t('rsvp.headcountContributorModeNumeric') }}</option>
                      <option value="attendance">{{ t('rsvp.headcountContributorModeAttendance') }}</option>
                    </select>
                    <input v-model="row.match_values_text" v-if="row.mode !== 'numeric'"
                           :class="[inputClass, 'col-span-6 sm:col-span-3']"
                           :placeholder="t('rsvp.headcountContributorMatchValuesPlaceholder')" />
                    <span v-else class="col-span-6 sm:col-span-3"></span>
                    <div class="col-span-12 sm:col-span-1 flex items-center justify-end gap-1">
                      <button type="button" class="text-muted-foreground hover:text-foreground disabled:opacity-30"
                              :disabled="idx === 0" :title="t('rsvp.headcountContributorMoveUp')"
                              @click="moveContributorRow(idx, -1)">
                        <ArrowUp class="h-3.5 w-3.5" />
                      </button>
                      <button type="button" class="text-muted-foreground hover:text-foreground disabled:opacity-30"
                              :disabled="idx === headcountContributors.length - 1" :title="t('rsvp.headcountContributorMoveDown')"
                              @click="moveContributorRow(idx, 1)">
                        <ArrowDown class="h-3.5 w-3.5" />
                      </button>
                      <button type="button" class="text-muted-foreground hover:text-red-500" :title="t('rsvp.headcountContributorRemove')"
                              @click="removeContributorRow(idx)">
                        <X class="h-3.5 w-3.5" />
                      </button>
                    </div>
                  </div>
                </div>
              </div>

              <div class="grid grid-cols-2 gap-4">
                <label class="block"><span class="text-sm">{{ t('rsvp.eventDate') }}</span>
                  <input type="date" v-model="form.event_date" :class="inputClass" /></label>
                <label class="block"><span class="text-sm">{{ t('rsvp.closeDate') }}</span>
                  <input type="date" v-model="form.rsvp_close_at" :class="inputClass" /></label>
              </div>

              <!-- WhatsApp account dropdown -->
              <div>
                <div class="flex items-center justify-between">
                  <span class="text-sm">{{ t('rsvp.account') }}</span>
                  <button type="button" class="text-xs text-muted-foreground hover:underline" @click="loadAccounts">
                    <RefreshCw class="inline h-3 w-3 mr-1" />{{ t('common.refresh') }}
                  </button>
                </div>
                <select v-model="form.whatsapp_account" :class="inputClass">
                  <option value="">{{ t('rsvp.selectAccount') }}</option>
                  <option v-for="a in accounts" :key="a.name" :value="a.name">{{ a.name }}</option>
                </select>
                <p v-if="!accounts.length" class="text-xs text-muted-foreground mt-1">
                  {{ t('rsvp.noAccountsHint') }}
                  <a href="/settings" target="_blank" class="underline">{{ t('nav.settings') }}</a>
                </p>
              </div>

              <!-- Question flow dropdown + one-click starter -->
              <div>
                <div class="flex items-center justify-between">
                  <span class="text-sm">{{ t('rsvp.flow') }}</span>
                  <div class="flex items-center gap-3">
                    <button type="button" class="text-xs text-muted-foreground hover:underline" @click="loadFlows">
                      <RefreshCw class="inline h-3 w-3 mr-1" />{{ t('common.refresh') }}
                    </button>
                    <button type="button" class="text-xs text-primary hover:underline disabled:opacity-50" :disabled="creatingFlow" @click="createStarterFlow">
                      <Sparkles class="inline h-3 w-3 mr-1" />{{ creatingFlow ? t('rsvp.creating') : t('rsvp.createStarterFlow') }}
                    </button>
                    <button type="button" class="text-xs text-primary hover:underline disabled:opacity-50" :disabled="generatingForm || !id" @click="generateForm">
                      <Sparkles class="inline h-3 w-3 mr-1" />{{ generatingForm ? t('rsvp.creating') : t('rsvp.generateFlowForm') }}
                    </button>
                  </div>
                </div>
                <select v-model="form.flow_id" :class="inputClass">
                  <option value="">{{ t('rsvp.selectFlow') }}</option>
                  <option v-for="fl in flows" :key="fl.id" :value="fl.id">{{ fl.name }}</option>
                </select>
                <p class="text-xs text-muted-foreground mt-1">{{ t('rsvp.flowHint') }}</p>
              </div>

              <!-- Invite template dropdown -->
              <div>
                <div class="flex items-center justify-between">
                  <span class="text-sm">{{ t('rsvp.inviteTemplate') }}</span>
                  <button type="button" class="text-xs text-muted-foreground hover:underline" @click="loadTemplates">
                    <RefreshCw class="inline h-3 w-3 mr-1" />{{ t('common.refresh') }}
                  </button>
                </div>
                <select v-model="form.template_id" :class="inputClass">
                  <option value="">{{ t('rsvp.selectTemplate') }}</option>
                  <option v-for="tpl in templates" :key="tpl.id" :value="tpl.id">{{ tpl.name }}</option>
                </select>
                <p v-if="!templates.length" class="text-xs text-muted-foreground mt-1">
                  {{ t('rsvp.noTemplatesHint') }}
                  <a href="/templates" target="_blank" class="underline">{{ t('nav.templates') }}</a>
                </p>
              </div>

              <div>
                <span class="text-sm">{{ t('rsvp.reminderTemplate') }}</span>
                <select v-model="form.reminder_template_id" :class="inputClass">
                  <option value="">{{ t('rsvp.selectTemplate') }}</option>
                  <option v-for="tpl in templates" :key="tpl.id" :value="tpl.id">{{ tpl.name }}</option>
                </select>
                <p class="text-xs text-muted-foreground mt-1">{{ t('rsvp.reminderTemplateHint') }}</p>
              </div>

              <div class="flex gap-2 pt-2">
                <Button :disabled="saving" @click="save">{{ t('rsvp.save') }}</Button>
                <Button v-if="id && status !== 'active'" variant="outline" @click="activate">{{ t('rsvp.activate') }}</Button>
                <Button v-if="id && status === 'active'" variant="outline" @click="close">{{ t('rsvp.close') }}</Button>
                <Button variant="ghost" @click="router.push('/rsvp')">{{ t('rsvp.cancel') }}</Button>
              </div>
              <p v-if="id" class="text-sm text-muted-foreground">{{ t('rsvp.status') }}: {{ status }}</p>
            </CardContent>
          </Card>

          <!-- Help / examples -->
          <Card>
            <CardContent class="pt-6">
              <button type="button" class="flex items-center gap-2 text-sm font-medium" @click="showHelp = !showHelp">
                <HelpCircle class="h-4 w-4" />{{ t('rsvp.helpTitle') }}
              </button>
              <div v-if="showHelp" class="mt-3 text-sm text-muted-foreground space-y-3">
                <div>
                  <p class="font-medium text-foreground">{{ t('rsvp.helpFlowTitle') }}</p>
                  <p>{{ t('rsvp.helpFlowBody') }}</p>
                  <pre class="mt-1 p-2 rounded bg-muted/40 text-xs whitespace-pre-wrap">Q1 (buttons, store as "attendance"): "Will you attend?"  -> Yes / No / Maybe
   Yes / Maybe -> Q2
Q2 (text, store as "headcount"): "How many people?"</pre>
                  <p class="mt-1">{{ t('rsvp.helpStarterHint') }}</p>
                </div>
                <div>
                  <p class="font-medium text-foreground">{{ t('rsvp.helpTemplateTitle') }}</p>
                  <p>{{ t('rsvp.helpTemplateBody') }}</p>
                  <pre class="mt-1 p-2 rounded bg-muted/40 text-xs whitespace-pre-wrap">Name: rsvp_invite
Body: {{ templateExampleBody }}
Button (quick reply): "RSVP now"</pre>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </ScrollArea>
  </div>
</template>
