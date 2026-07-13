<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { rsvpService } from '@/services/api'
import { toast } from 'vue-sonner'
import { PageHeader, DataTable, DeleteConfirmDialog, SearchInput, RefreshButton, type Column } from '@/components/shared'
import { getErrorMessage } from '@/lib/api-utils'
import { formatDateDDMMYYYY } from '@/lib/utils'
import { Plus, Pencil, Trash2, BarChart3, CalendarCheck } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'

interface RSVPEvent { id: string; name: string; status: string; event_date?: string }

const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()

const events = ref<RSVPEvent[]>([])
const tallies = ref<Record<string, Record<string, number>>>({})
const isLoading = ref(true)
const searchQuery = ref('')
const deleteDialogOpen = ref(false)
const toDelete = ref<RSVPEvent | null>(null)

const columns = computed<Column<RSVPEvent>[]>(() => [
  { key: 'name', label: t('rsvp.name'), sortable: true },
  { key: 'status', label: t('rsvp.status') },
  { key: 'event_date', label: t('rsvp.eventDate') },
  { key: 'responses', label: t('rsvp.responses') },
  { key: 'actions', label: t('rsvp.actions'), align: 'right' },
])

const filtered = computed(() => {
  const q = searchQuery.value.toLowerCase().trim()
  if (!q) return events.value
  return events.value.filter(e => e.name.toLowerCase().includes(q))
})

async function fetchEvents() {
  isLoading.value = true
  try {
    const res = await rsvpService.list()
    const data = (res.data as any).data || res.data
    events.value = data.events || []
    await loadTallies()
  } catch {
    events.value = []
  } finally {
    isLoading.value = false
  }
}

async function loadTallies() {
  const results = await Promise.all(events.value.map(async (e) => {
    try {
      const r = await rsvpService.tally(e.id)
      return [e.id, (r.data as any).data || r.data] as const
    } catch {
      return [e.id, {}] as const
    }
  }))
  tallies.value = Object.fromEntries(results)
}
onMounted(fetchEvents)

function createEvent() { router.push('/rsvp/new') }
function editEvent(e: RSVPEvent) { router.push(`/rsvp/${e.id}/edit`) }
function viewResults(e: RSVPEvent) { router.push(`/rsvp/${e.id}/results`) }
function openDeleteDialog(e: RSVPEvent) { toDelete.value = e; deleteDialogOpen.value = true }

async function confirmDelete() {
  if (!toDelete.value) return
  try {
    await rsvpService.delete(toDelete.value.id)
    toast.success(t('rsvp.delete'))
    deleteDialogOpen.value = false
    toDelete.value = null
    await fetchEvents()
  } catch (e: any) {
    toast.error(getErrorMessage(e, t('rsvp.delete')))
  }
}
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="t('rsvp.title')" :icon="CalendarCheck" back-link="/">
      <template #actions>
        <RefreshButton :refreshing="isLoading" :label="t('common.refresh')" @refresh="fetchEvents" />
        <Button v-if="auth.hasPermission('rsvp', 'write')" variant="outline" size="sm" @click="createEvent">
          <Plus class="h-4 w-4 mr-2" />
          {{ t('rsvp.create') }}
        </Button>
      </template>
    </PageHeader>

    <ScrollArea class="flex-1">
      <div class="p-6">
        <div class="max-w-6xl mx-auto">
          <Card>
            <CardHeader>
              <div class="flex items-center justify-between flex-wrap gap-4">
                <div>
                  <CardTitle>{{ t('rsvp.yourEvents') }}</CardTitle>
                  <CardDescription>{{ t('rsvp.subtitle') }}</CardDescription>
                </div>
                <SearchInput v-model="searchQuery" :placeholder="t('rsvp.search') + '...'" class="w-64" />
              </div>
            </CardHeader>
            <CardContent>
              <DataTable
                :items="filtered"
                :columns="columns"
                :is-loading="isLoading"
                :empty-icon="CalendarCheck"
                :empty-title="searchQuery ? t('rsvp.noMatching') : t('rsvp.noEvents')"
                :empty-description="searchQuery ? '' : t('rsvp.noEventsDesc')"
                item-name="events"
              >
                <template #cell-name="{ item }">
                  <span class="font-medium">{{ item.name }}</span>
                </template>
                <template #cell-status="{ item }">
                  <Badge variant="secondary" class="text-xs">{{ t('rsvp.' + item.status) !== 'rsvp.' + item.status ? t('rsvp.' + item.status) : item.status }}</Badge>
                </template>
                <template #cell-event_date="{ item }">
                  <span class="text-sm text-muted-foreground">{{ item.event_date ? formatDateDDMMYYYY(item.event_date) : '—' }}</span>
                </template>
                <template #cell-responses="{ item }">
                  <div class="flex items-center gap-2 text-xs">
                    <span class="text-green-600 font-medium" :title="t('rsvp.yes')">{{ tallies[item.id]?.yes ?? 0 }} ✓</span>
                    <span class="text-red-600 font-medium" :title="t('rsvp.no')">{{ tallies[item.id]?.no ?? 0 }} ✗</span>
                    <span class="text-amber-600 font-medium" :title="t('rsvp.maybe')">{{ tallies[item.id]?.maybe ?? 0 }} ~</span>
                    <span class="text-muted-foreground" :title="t('rsvp.pending')">{{ tallies[item.id]?.pending ?? 0 }} ⧗</span>
                    <span class="font-semibold" :title="t('rsvp.total')">Σ {{ tallies[item.id]?.total ?? 0 }}</span>
                  </div>
                </template>
                <template #cell-actions="{ item }">
                  <div class="flex items-center justify-end gap-1">
                    <Button variant="ghost" size="sm" class="h-8 gap-1.5" :title="t('rsvp.results')" @click="viewResults(item)">
                      <BarChart3 class="h-4 w-4" />
                      <span class="text-xs">{{ t('rsvp.results') }}</span>
                    </Button>
                    <Button v-if="auth.hasPermission('rsvp', 'write')" variant="ghost" size="sm" class="h-8 gap-1.5" :title="t('common.edit')" @click="editEvent(item)">
                      <Pencil class="h-4 w-4" />
                      <span class="text-xs">{{ t('common.edit') }}</span>
                    </Button>
                    <Button v-if="auth.hasPermission('rsvp', 'delete')" variant="ghost" size="sm" class="h-8 gap-1.5 text-destructive" :title="t('common.delete')" @click="openDeleteDialog(item)">
                      <Trash2 class="h-4 w-4" />
                      <span class="text-xs">{{ t('common.delete') }}</span>
                    </Button>
                  </div>
                </template>
                <template #empty-action>
                  <Button v-if="!searchQuery && auth.hasPermission('rsvp', 'write')" variant="outline" size="sm" @click="createEvent">
                    <Plus class="h-4 w-4 mr-2" />
                    {{ t('rsvp.create') }}
                  </Button>
                </template>
              </DataTable>
            </CardContent>
          </Card>
        </div>
      </div>
    </ScrollArea>

    <DeleteConfirmDialog
      v-model:open="deleteDialogOpen"
      :title="t('rsvp.deletePrompt')"
      :item-name="toDelete?.name"
      @confirm="confirmDelete"
    />
  </div>
</template>
