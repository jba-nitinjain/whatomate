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
import { rsvpService } from '@/services/api'
import { CalendarCheck } from 'lucide-vue-next'

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
onMounted(load)

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

const inputClass = 'w-full border rounded px-2 py-1 bg-transparent text-sm'
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="id ? t('rsvp.editTitle') : t('rsvp.newTitle')" :icon="CalendarCheck" back-link="/rsvp" />

    <ScrollArea class="flex-1">
      <div class="p-6">
        <div class="max-w-2xl mx-auto">
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

              <label class="block"><span class="text-sm">{{ t('rsvp.account') }}</span>
                <input v-model="form.whatsapp_account" :class="inputClass" /></label>
              <label class="block"><span class="text-sm">{{ t('rsvp.flowId') }}</span>
                <input v-model="form.flow_id" :class="inputClass" /></label>
              <label class="block"><span class="text-sm">{{ t('rsvp.inviteTemplate') }}</span>
                <input v-model="form.template_id" :class="inputClass" /></label>

              <label class="flex items-center gap-2">
                <input type="checkbox" v-model="form.reminder_enabled" /> <span class="text-sm">{{ t('rsvp.reminder') }}</span>
              </label>
              <div v-if="form.reminder_enabled" class="grid grid-cols-2 gap-4">
                <label class="block"><span class="text-sm">{{ t('rsvp.reminderAt') }}</span>
                  <input type="datetime-local" v-model="form.reminder_at" :class="inputClass" /></label>
                <label class="block"><span class="text-sm">{{ t('rsvp.reminderTemplate') }}</span>
                  <input v-model="form.reminder_template_id" :class="inputClass" /></label>
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
        </div>
      </div>
    </ScrollArea>
  </div>
</template>
