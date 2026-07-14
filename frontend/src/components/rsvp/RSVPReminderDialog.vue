<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { rsvpService, templatesService } from '@/services/api'
import { formatDateTimeIST } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Loader2, Send, Trash2 } from 'lucide-vue-next'

interface Schedule { id: string; scheduled_at: string; template_id: string; status: string; sent_count: number; failed_count: number }
const props = defineProps<{ open: boolean; eventId: string; selectedIds: string[] }>()
const emit = defineEmits<{ 'update:open': [value: boolean]; changed: [] }>()
const { t } = useI18n()
const schedules = ref<Schedule[]>([])
const templates = ref<any[]>([])
const templateId = ref('')
const scheduledAt = ref('')
const loading = ref(false)

function unwrap(response: any, key: string) { const data = response?.data?.data || response?.data || {}; return data[key] || [] }
async function load() {
  loading.value = true
  try {
    const [eventResponse, scheduleResponse, templateResponse] = await Promise.all([rsvpService.get(props.eventId), rsvpService.listReminders(props.eventId), templatesService.list({ status: 'approved', limit: 200 })])
    const event = (eventResponse.data as any).data || eventResponse.data
    schedules.value = unwrap(scheduleResponse, 'reminders')
    templates.value = unwrap(templateResponse, 'templates')
    templateId.value ||= event.reminder_template_id || ''
  } finally { loading.value = false }
}
watch(() => props.open, value => { if (value) load() })

async function send(all: boolean) {
  if (!templateId.value) { toast.error(t('rsvp.selectReminderTemplate')); return }
  loading.value = true
  try {
    const response = await rsvpService.sendReminders(props.eventId, all ? { all_not_started: true, template_id: templateId.value } : { response_ids: props.selectedIds, template_id: templateId.value })
    const data = (response.data as any).data || response.data
    toast.success(t('rsvp.reminderResult', { sent: data.sent || 0, skipped: data.skipped || 0, failed: data.failed || 0 }))
    emit('changed')
  } catch (error: any) { toast.error(error?.response?.data?.message || t('rsvp.reminderFailed')) }
  finally { loading.value = false }
}
async function schedule() {
  if (!scheduledAt.value || !templateId.value) return
  loading.value = true
  try {
    await rsvpService.createReminder(props.eventId, { scheduled_at: new Date(scheduledAt.value).toISOString(), template_id: templateId.value })
    scheduledAt.value = ''; toast.success(t('rsvp.reminderScheduled')); await load()
  } catch (error: any) { toast.error(error?.response?.data?.message || t('rsvp.reminderFailed')) }
  finally { loading.value = false }
}
async function cancel(item: Schedule) { await rsvpService.cancelReminder(props.eventId, item.id); await load() }
</script>

<template>
  <Dialog :open="open" @update:open="emit('update:open', $event)">
    <DialogContent class="max-w-2xl">
      <DialogHeader><DialogTitle>{{ t('rsvp.remindersTitle') }}</DialogTitle><DialogDescription>{{ t('rsvp.remindersHint') }}</DialogDescription></DialogHeader>
      <div v-if="loading" class="flex justify-center p-4"><Loader2 class="h-5 w-5 animate-spin" /></div>
      <div class="space-y-5">
        <label class="block text-sm"><span>{{ t('rsvp.reminderTemplate') }}</span><select v-model="templateId" class="mt-1 w-full rounded border bg-transparent px-2 py-2"><option value="">{{ t('rsvp.selectTemplate') }}</option><option v-for="tpl in templates" :key="tpl.id" :value="tpl.id">{{ tpl.name }}</option></select></label>
        <div class="grid gap-2 sm:grid-cols-2"><Button :disabled="loading || !selectedIds.length" @click="send(false)"><Send class="mr-2 h-4 w-4" />{{ t('rsvp.remindSelected', { count: selectedIds.length }) }}</Button><Button variant="outline" :disabled="loading" @click="send(true)">{{ t('rsvp.remindAllNotStarted') }}</Button></div>
        <div class="rounded-lg border p-4 space-y-3"><div class="font-medium">{{ t('rsvp.scheduleReminder') }}</div><div class="flex gap-2"><Input v-model="scheduledAt" type="datetime-local" /><Button :disabled="!scheduledAt || !templateId || loading" @click="schedule">{{ t('rsvp.schedule') }}</Button></div></div>
        <div class="space-y-2"><div class="font-medium">{{ t('rsvp.scheduledReminders') }}</div><p v-if="!schedules.length" class="text-sm text-muted-foreground">{{ t('rsvp.noScheduledReminders') }}</p><div v-for="item in schedules" :key="item.id" class="flex items-center justify-between rounded-lg border p-3 text-sm"><div><div>{{ formatDateTimeIST(item.scheduled_at) }}</div><div class="text-xs text-muted-foreground">{{ item.status }} · {{ item.sent_count }} sent · {{ item.failed_count }} failed</div></div><Button v-if="item.status === 'pending'" variant="ghost" size="icon" @click="cancel(item)"><Trash2 class="h-4 w-4" /></Button></div></div>
      </div>
      <DialogFooter><Button variant="outline" @click="emit('update:open', false)">{{ t('common.close') }}</Button></DialogFooter>
    </DialogContent>
  </Dialog>
</template>
