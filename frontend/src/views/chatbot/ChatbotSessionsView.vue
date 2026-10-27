<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { PageHeader, DataTable, ErrorState, RefreshButton, type Column } from '@/components/shared'
import SessionFilters from '@/components/chatbot/sessions/SessionFilters.vue'
import SessionStatusBadge from '@/components/chatbot/sessions/SessionStatusBadge.vue'
import { chatbotService, chatbotSessionsService, type ChatbotSession } from '@/services/api'
import { useSearchPagination } from '@/composables/useSearchPagination'
import { useViewRefresh } from '@/composables/useViewRefresh'
import type { DateRangeResult } from '@/composables/useDateRange'
import { getErrorMessage } from '@/lib/api-utils'
import { formatDateTime } from '@/lib/utils'
import { toast } from 'vue-sonner'
import { History } from 'lucide-vue-next'

const { t } = useI18n()

const sessions = ref<ChatbotSession[]>([])
const flows = ref<{ id: string; name: string }[]>([])
const isLoading = ref(true)
const error = ref<string | null>(null)

const filterStatus = ref('all')
const filterFlow = ref('all')
const range = ref<DateRangeResult>()

const sortKey = ref('last_activity_at')
const sortDirection = ref<'asc' | 'desc'>('desc')

const breadcrumbs = computed(() => [
  { label: t('nav.chatbot'), href: '/chatbot' },
  { label: t('chatbotSessions.title') },
])

const columns = computed<Column<ChatbotSession>[]>(() => [
  { key: 'contact', label: t('chatbotSessions.contact') },
  { key: 'flow_name', label: t('chatbotSessions.flow') },
  { key: 'status', label: t('chatbotSessions.status') },
  { key: 'current_step', label: t('chatbotSessions.currentStep') },
  { key: 'message_count', label: t('chatbotSessions.messages'), align: 'center' },
  { key: 'started_at', label: t('chatbotSessions.startedAt') },
  { key: 'last_activity_at', label: t('chatbotSessions.lastActivity') },
])

async function fetchSessions() {
  isLoading.value = true
  error.value = null
  try {
    const response = await chatbotSessionsService.list({
      search: searchQuery.value || undefined,
      status: filterStatus.value !== 'all' ? filterStatus.value : undefined,
      flow_id: filterFlow.value !== 'all' ? filterFlow.value : undefined,
      from: range.value?.from,
      to: range.value?.to,
      page: currentPage.value,
      limit: pageSize,
    })
    const data = (response.data as any).data || response.data
    sessions.value = data.sessions || []
    totalItems.value = data.total ?? sessions.value.length
  } catch (err) {
    const message = getErrorMessage(err, t('chatbotSessions.fetchError'))
    toast.error(message)
    error.value = message
  } finally {
    isLoading.value = false
  }
}

async function fetchFlows() {
  try {
    const response = await chatbotService.listFlows({ limit: 100 })
    const data = (response.data as any).data || response.data
    flows.value = (data.flows || []).map((f: any) => ({ id: f.id, name: f.name }))
  } catch {
    flows.value = []
  }
}

const { searchQuery, currentPage, totalItems, pageSize, handlePageChange, resetAndFetch } = useSearchPagination({
  pageSize: 25,
  fetchFn: () => fetchSessions(),
})
const { isRefreshing, refreshNow } = useViewRefresh(fetchSessions)

watch([filterStatus, filterFlow], () => resetAndFetch())
// The filter bar publishes its initial range during setup (before onMounted fetches),
// so only later range changes trigger a refetch.
watch(range, (_next, prev) => {
  if (prev) resetAndFetch()
})

function contactName(session: ChatbotSession): string {
  return session.contact?.profile_name || session.contact?.phone_number || session.phone_number || t('chatbotSessions.unknownContact')
}

onMounted(() => {
  fetchFlows()
  fetchSessions()
})
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader
      :title="t('chatbotSessions.title')"
      :description="t('chatbotSessions.description')"
      :icon="History"
      icon-gradient="bg-gradient-to-br from-violet-500 to-purple-600 shadow-violet-500/20"
      :breadcrumbs="breadcrumbs"
    >
      <template #actions>
        <RefreshButton :refreshing="isRefreshing" :label="t('common.refresh')" @refresh="refreshNow(true)" />
      </template>
    </PageHeader>

    <ErrorState
      v-if="error && !isLoading"
      :title="t('common.loadErrorTitle')"
      :description="error"
      :retry-label="t('common.retry')"
      class="flex-1"
      @retry="fetchSessions"
    />

    <ScrollArea v-else class="flex-1">
      <div class="p-6">
        <Card>
          <CardHeader>
            <div class="flex items-center justify-between flex-wrap gap-4">
              <div>
                <CardTitle>{{ t('chatbotSessions.allConversations') }}</CardTitle>
                <CardDescription>{{ t('chatbotSessions.allConversationsDesc') }}</CardDescription>
              </div>
              <SessionFilters
                v-model:status="filterStatus"
                v-model:flow-id="filterFlow"
                v-model:search="searchQuery"
                v-model:range="range"
                :flows="flows"
              />
            </div>
          </CardHeader>
          <CardContent>
            <DataTable
              :items="sessions"
              :columns="columns"
              :is-loading="isLoading"
              :empty-icon="History"
              :empty-title="t('chatbotSessions.noSessions')"
              :empty-description="t('chatbotSessions.noSessionsDesc')"
              v-model:sort-key="sortKey"
              v-model:sort-direction="sortDirection"
              server-pagination
              :current-page="currentPage"
              :total-items="totalItems"
              :page-size="pageSize"
              :item-name="t('chatbotSessions.itemName')"
              @page-change="handlePageChange"
            >
              <template #cell-contact="{ item }">
                <RouterLink
                  :to="`/chatbot/sessions/${item.id}`"
                  class="block py-1 text-inherit no-underline hover:opacity-80"
                  data-testid="session-row-link"
                >
                  <span class="font-medium">{{ contactName(item) }}</span>
                  <span v-if="item.contact?.profile_name" class="block text-xs text-muted-foreground">
                    {{ item.phone_number }}
                  </span>
                </RouterLink>
              </template>
              <template #cell-flow_name="{ item }">
                <span class="text-sm" :class="item.flow_name ? '' : 'text-muted-foreground'">
                  {{ item.flow_name || t('chatbotSessions.noFlow') }}
                </span>
              </template>
              <template #cell-status="{ item }">
                <SessionStatusBadge :status="item.status" />
              </template>
              <template #cell-current_step="{ item }">
                <span class="text-sm text-muted-foreground">{{ item.current_step || '—' }}</span>
              </template>
              <template #cell-message_count="{ item }">
                <span class="text-sm">{{ item.message_count }}</span>
              </template>
              <template #cell-started_at="{ item }">
                <span class="text-sm text-muted-foreground">{{ formatDateTime(item.started_at) }}</span>
              </template>
              <template #cell-last_activity_at="{ item }">
                <span class="text-sm text-muted-foreground">{{ formatDateTime(item.last_activity_at) }}</span>
              </template>
            </DataTable>
          </CardContent>
        </Card>
      </div>
    </ScrollArea>
  </div>
</template>
