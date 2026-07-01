<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { rsvpService } from '@/services/api'
import { formatDateDDMMYYYY } from '@/lib/utils'

const route = useRoute()
const id = route.params.id as string
const tally = ref<Record<string, number>>({ yes: 0, no: 0, maybe: 0, pending: 0, total: 0 })
const responses = ref<any[]>([])
let timer: number | undefined
const labels: Record<string, string> = { yes: 'Yes', no: 'No', maybe: 'Maybe', pending: 'Pending', total: 'Total' }
const cards = ['yes', 'no', 'maybe', 'pending', 'total']

async function loadTally() {
  const r = await rsvpService.tally(id)
  tally.value = (r.data as any).data || r.data
}
async function loadResponses() {
  const r = await rsvpService.responses(id)
  const d = (r.data as any).data || r.data
  responses.value = d.responses || []
}
function exportXlsx() {
  window.open(rsvpService.exportUrl(id), '_blank')
}

onMounted(async () => {
  await Promise.all([loadTally(), loadResponses()])
  timer = window.setInterval(loadTally, 15000)
})
onUnmounted(() => { if (timer) window.clearInterval(timer) })
</script>

<template>
  <div class="p-6 max-w-6xl mx-auto space-y-6">
    <div class="flex items-center justify-between">
      <h1 class="text-xl font-semibold">RSVP Results</h1>
      <Button variant="outline" size="sm" @click="exportXlsx">Export</Button>
    </div>

    <div class="grid grid-cols-2 md:grid-cols-5 gap-4">
      <Card v-for="k in cards" :key="k">
        <CardHeader><CardTitle class="text-sm text-muted-foreground">{{ labels[k] }}</CardTitle></CardHeader>
        <CardContent><div class="text-2xl font-bold">{{ tally[k] ?? 0 }}</div></CardContent>
      </Card>
    </div>

    <table class="w-full text-sm">
      <thead>
        <tr class="text-left text-muted-foreground border-b">
          <th class="py-2">Guest</th><th>Status</th><th>Responded</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in responses" :key="row.id" class="border-b">
          <td class="py-2">{{ row.contact?.profile_name || row.phone_number }}</td>
          <td>{{ labels[row.attendance] || row.attendance }}</td>
          <td>{{ row.responded_at ? formatDateDDMMYYYY(row.responded_at) : '—' }}</td>
        </tr>
        <tr v-if="!responses.length">
          <td colspan="3" class="py-6 text-center text-muted-foreground">No responses yet.</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
