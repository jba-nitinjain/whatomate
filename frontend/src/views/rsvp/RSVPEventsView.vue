<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { Button } from '@/components/ui/button'
import { rsvpService } from '@/services/api'
import { formatDateDDMMYYYY } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth'

interface RSVPEvent { id: string; name: string; status: string; event_date?: string }

const router = useRouter()
const auth = useAuthStore()
const events = ref<RSVPEvent[]>([])
const loading = ref(true)

async function fetchEvents() {
  loading.value = true
  try {
    const res = await rsvpService.list()
    const data = (res.data as any).data || res.data
    events.value = data.events || []
  } catch {
    events.value = []
  } finally {
    loading.value = false
  }
}
onMounted(fetchEvents)

async function remove(e: RSVPEvent) {
  if (!confirm(`Delete RSVP "${e.name}"?`)) return
  await rsvpService.delete(e.id)
  await fetchEvents()
}
</script>

<template>
  <div class="p-6 max-w-6xl mx-auto">
    <div class="flex items-center justify-between mb-4">
      <h1 class="text-xl font-semibold">RSVP Events</h1>
      <Button v-if="auth.hasPermission('rsvp', 'write')" size="sm" @click="router.push('/rsvp/new')">New RSVP</Button>
    </div>
    <div v-if="loading" class="text-muted-foreground">Loading…</div>
    <table v-else class="w-full text-sm">
      <thead>
        <tr class="text-left text-muted-foreground border-b">
          <th class="py-2">Name</th><th>Status</th><th>Event date</th><th class="text-right">Actions</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="e in events" :key="e.id" class="border-b">
          <td class="py-2">{{ e.name }}</td>
          <td>{{ e.status }}</td>
          <td>{{ e.event_date ? formatDateDDMMYYYY(e.event_date) : '—' }}</td>
          <td class="text-right space-x-2">
            <Button variant="ghost" size="sm" @click="router.push(`/rsvp/${e.id}/results`)">Results</Button>
            <Button v-if="auth.hasPermission('rsvp', 'write')" variant="ghost" size="sm" @click="router.push(`/rsvp/${e.id}/edit`)">Edit</Button>
            <Button v-if="auth.hasPermission('rsvp', 'delete')" variant="ghost" size="sm" @click="remove(e)">Delete</Button>
          </td>
        </tr>
        <tr v-if="!events.length">
          <td colspan="4" class="py-6 text-center text-muted-foreground">No RSVP events yet.</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
