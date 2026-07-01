<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Button } from '@/components/ui/button'
import { rsvpService } from '@/services/api'

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
    router.push('/rsvp')
  } finally {
    saving.value = false
  }
}

async function activate() {
  if (!id.value) return
  try {
    await rsvpService.activate(id.value)
    await load()
  } catch (e: any) {
    alert(e?.response?.data?.message || 'Activate failed (keyword may already be in use by another active event)')
  }
}

async function close() {
  if (!id.value) return
  await rsvpService.close(id.value)
  await load()
}
</script>

<template>
  <div class="p-6 max-w-2xl mx-auto space-y-4">
    <h1 class="text-xl font-semibold">{{ id ? 'Edit' : 'New' }} RSVP</h1>

    <label class="block"><span class="text-sm">Name</span>
      <input v-model="form.name" class="w-full border rounded px-2 py-1 bg-transparent" /></label>
    <label class="block"><span class="text-sm">Description</span>
      <textarea v-model="form.description" class="w-full border rounded px-2 py-1 bg-transparent"></textarea></label>
    <label class="block"><span class="text-sm">Keyword (unique per active event)</span>
      <input v-model="form.keyword" class="w-full border rounded px-2 py-1 bg-transparent" /></label>

    <div class="grid grid-cols-2 gap-4">
      <label class="block"><span class="text-sm">Event date</span>
        <input type="date" v-model="form.event_date" class="w-full border rounded px-2 py-1 bg-transparent" /></label>
      <label class="block"><span class="text-sm">RSVP close date</span>
        <input type="date" v-model="form.rsvp_close_at" class="w-full border rounded px-2 py-1 bg-transparent" /></label>
    </div>

    <label class="block"><span class="text-sm">WhatsApp account name</span>
      <input v-model="form.whatsapp_account" class="w-full border rounded px-2 py-1 bg-transparent" /></label>
    <label class="block"><span class="text-sm">Question flow ID (chatbot flow UUID)</span>
      <input v-model="form.flow_id" class="w-full border rounded px-2 py-1 bg-transparent" /></label>
    <label class="block"><span class="text-sm">Invite template ID</span>
      <input v-model="form.template_id" class="w-full border rounded px-2 py-1 bg-transparent" /></label>

    <label class="flex items-center gap-2">
      <input type="checkbox" v-model="form.reminder_enabled" /> <span class="text-sm">Send reminders to non-responders</span>
    </label>
    <div v-if="form.reminder_enabled" class="grid grid-cols-2 gap-4">
      <label class="block"><span class="text-sm">Reminder time</span>
        <input type="datetime-local" v-model="form.reminder_at" class="w-full border rounded px-2 py-1 bg-transparent" /></label>
      <label class="block"><span class="text-sm">Reminder template ID</span>
        <input v-model="form.reminder_template_id" class="w-full border rounded px-2 py-1 bg-transparent" /></label>
    </div>

    <div class="flex gap-2 pt-2">
      <Button :disabled="saving" @click="save">Save</Button>
      <Button v-if="id && status !== 'active'" variant="outline" @click="activate">Activate</Button>
      <Button v-if="id && status === 'active'" variant="outline" @click="close">Close</Button>
      <Button variant="ghost" @click="router.push('/rsvp')">Cancel</Button>
    </div>
    <p v-if="id" class="text-sm text-muted-foreground">Status: {{ status }}</p>
  </div>
</template>
