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
import { CalendarCheck, RefreshCw, Sparkles, HelpCircle } from 'lucide-vue-next'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const id = computed(() => route.params.id as string | undefined)

const form = ref<any>({
  name: '', description: '', keyword: '', event_date: '', rsvp_close_at: '',
  whatsapp_account: '', flow_id: '', template_id: '', reminder_enabled: false,
  reminder_at: '', reminder_template_id: ''
})
const status = ref('draft')
const saving = ref(false)
const creatingFlow = ref(false)
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
    const list = unwrap(await templatesService.list({ status: 'approved', limit: 200 }), 'templates')
    templates.value = list.length ? list : unwrap(await templatesService.list({ limit: 200 }), 'templates')
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
    reminder_template_id: e.reminder_template_id || ''
  }
  status.value = e.status || 'draft'
}

onMounted(async () => {
  await Promise.all([loadAccounts(), loadFlows(), loadTemplates(), load()])
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
    reminder_template_id: f.reminder_template_id || null
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
          message: 'Will you attend our event?',
          message_type: 'buttons',
          buttons: [
            { id: 'yes', title: 'Yes' },
            { id: 'no', title: 'No' },
            { id: 'maybe', title: 'Maybe' }
          ],
          store_as: 'attendance',
          conditional_next: { yes: 'headcount', maybe: 'headcount', default: '' }
        },
        {
          step_name: 'headcount',
          message: 'How many people will attend (including you)?',
          message_type: 'text',
          store_as: 'headcount'
        }
      ]
    }
    const res = await chatbotService.createFlow(body)
    const created = (res.data as any).data || res.data
    const newId = created?.id || created?.flow?.id || created?.flow_id
    await loadFlows()
    if (newId) form.value.flow_id = newId
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

              <label class="flex items-center gap-2">
                <input type="checkbox" v-model="form.reminder_enabled" /> <span class="text-sm">{{ t('rsvp.reminder') }}</span>
              </label>
              <div v-if="form.reminder_enabled" class="grid grid-cols-2 gap-4">
                <label class="block"><span class="text-sm">{{ t('rsvp.reminderAt') }}</span>
                  <input type="datetime-local" v-model="form.reminder_at" :class="inputClass" /></label>
                <div>
                  <span class="text-sm">{{ t('rsvp.reminderTemplate') }}</span>
                  <select v-model="form.reminder_template_id" :class="inputClass">
                    <option value="">{{ t('rsvp.selectTemplate') }}</option>
                    <option v-for="tpl in templates" :key="tpl.id" :value="tpl.id">{{ tpl.name }}</option>
                  </select>
                </div>
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
