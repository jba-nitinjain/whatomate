<script setup lang="ts">
import { ref, onMounted, watch, computed } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PageHeader, SearchInput, DataTable, IconButton, DeleteConfirmDialog, ErrorState, RefreshButton, type Column } from '@/components/shared'
import { api, templatesService } from '@/services/api'
import { useAuthStore } from '@/stores/auth'
import { useOrganizationsStore } from '@/stores/organizations'
import { toast } from 'vue-sonner'
import { Plus, RefreshCw, FileText, Pencil, Trash2, Loader2, MessageSquare, Image, FileIcon, Video, Megaphone } from 'lucide-vue-next'
import { getErrorMessage } from '@/lib/api-utils'
import { useSearchPagination } from '@/composables/useSearchPagination'
import { useViewRefresh } from '@/composables/useViewRefresh'
import { useDebounceFn } from '@vueuse/core'
import { openCampaignFromTemplate } from '@/utils/templateCampaign'

const { t } = useI18n()
const router = useRouter()
const authStore = useAuthStore()

interface WhatsAppAccount {
  id: string
  name: string
  phone_id: string
}

interface Template {
  id: string
  whatsapp_account: string
  meta_template_id: string
  name: string
  display_name: string
  language: string
  category: string
  status: string
  header_type: string
  header_content: string
  body_content: string
  footer_content: string
  buttons: any[]
  sample_values: any[]
  created_at: string
  updated_at: string
}

const organizationsStore = useOrganizationsStore()

const templates = ref<Template[]>([])
const accounts = ref<WhatsAppAccount[]>([])
const isLoading = ref(true)
const error = ref<string | null>(null)
const isSyncing = ref(false)
const selectedAccount = ref<string>(localStorage.getItem('templates_selected_account') || 'all')
const selectedStatus = ref<string>(localStorage.getItem('templates_selected_status') || 'all')
const selectedCategory = ref<string>(localStorage.getItem('templates_selected_category') || 'all')
const canCreateCampaign = computed(() => authStore.hasPermission('campaigns', 'write'))
const templateStatusCounts = ref<Record<string, number>>({ all: 0 })
const templateCategoryCounts = ref<Record<string, number>>({ all: 0 })

// Delete dialog state
const deleteDialogOpen = ref(false)
const templateToDelete = ref<Template | null>(null)
const isDeleting = ref(false)

const { searchQuery, currentPage, totalItems, pageSize, handlePageChange } = useSearchPagination({
  fetchFn: () => fetchTemplates(),
})

const columns = computed<Column<Template>[]>(() => [
  { key: 'name', label: t('templates.name'), sortable: true },
  { key: 'category', label: t('templates.category'), sortable: true },
  { key: 'status', label: t('templates.status'), sortable: true },
  { key: 'language', label: t('templates.language'), sortable: true },
  { key: 'header_type', label: t('templates.header') },
  { key: 'actions', label: '', align: 'right' },
])

const sortKey = ref('name')
const sortDirection = ref<'asc' | 'desc'>('asc')

const statusOptions = [
  'APPROVED',
  'PENDING',
  'REJECTED',
  'DRAFT',
  'DISABLED',
  'PENDING_DELETION',
  'DELETED',
  'REINSTATED',
  'FLAGGED',
]

const statusTabs = ['all', ...statusOptions]

const categories = [
  { value: 'UTILITY', label: 'Utility' },
  { value: 'MARKETING', label: 'Marketing' },
  { value: 'AUTHENTICATION', label: 'Authentication' },
]

const categoryTabs = ['all', ...categories.map(category => category.value)]

const languages = [
  { code: 'af', name: 'Afrikaans' },
  { code: 'sq', name: 'Albanian' },
  { code: 'ar', name: 'Arabic' },
  { code: 'az', name: 'Azerbaijani' },
  { code: 'bn', name: 'Bengali' },
  { code: 'bg', name: 'Bulgarian' },
  { code: 'ca', name: 'Catalan' },
  { code: 'zh_CN', name: 'Chinese (CHN)' },
  { code: 'zh_HK', name: 'Chinese (HKG)' },
  { code: 'zh_TW', name: 'Chinese (TAI)' },
  { code: 'hr', name: 'Croatian' },
  { code: 'cs', name: 'Czech' },
  { code: 'da', name: 'Danish' },
  { code: 'nl', name: 'Dutch' },
  { code: 'en', name: 'English' },
  { code: 'en_GB', name: 'English (UK)' },
  { code: 'en_US', name: 'English (US)' },
  { code: 'et', name: 'Estonian' },
  { code: 'fil', name: 'Filipino' },
  { code: 'fi', name: 'Finnish' },
  { code: 'fr', name: 'French' },
  { code: 'ka', name: 'Georgian' },
  { code: 'de', name: 'German' },
  { code: 'el', name: 'Greek' },
  { code: 'gu', name: 'Gujarati' },
  { code: 'ha', name: 'Hausa' },
  { code: 'he', name: 'Hebrew' },
  { code: 'hi', name: 'Hindi' },
  { code: 'hu', name: 'Hungarian' },
  { code: 'id', name: 'Indonesian' },
  { code: 'ga', name: 'Irish' },
  { code: 'it', name: 'Italian' },
  { code: 'ja', name: 'Japanese' },
  { code: 'kn', name: 'Kannada' },
  { code: 'kk', name: 'Kazakh' },
  { code: 'rw_RW', name: 'Kinyarwanda' },
  { code: 'ko', name: 'Korean' },
  { code: 'ky_KG', name: 'Kyrgyz (Kyrgyzstan)' },
  { code: 'lo', name: 'Lao' },
  { code: 'lv', name: 'Latvian' },
  { code: 'lt', name: 'Lithuanian' },
  { code: 'mk', name: 'Macedonian' },
  { code: 'ms', name: 'Malay' },
  { code: 'ml', name: 'Malayalam' },
  { code: 'mr', name: 'Marathi' },
  { code: 'nb', name: 'Norwegian (Bokm\u00e5l)' },
  { code: 'fa', name: 'Persian' },
  { code: 'pl', name: 'Polish' },
  { code: 'pt_BR', name: 'Portuguese (BR)' },
  { code: 'pt_PT', name: 'Portuguese (POR)' },
  { code: 'pa', name: 'Punjabi' },
  { code: 'ro', name: 'Romanian' },
  { code: 'ru', name: 'Russian' },
  { code: 'sr', name: 'Serbian' },
  { code: 'sk', name: 'Slovak' },
  { code: 'sl', name: 'Slovenian' },
  { code: 'es', name: 'Spanish' },
  { code: 'es_AR', name: 'Spanish (ARG)' },
  { code: 'es_MX', name: 'Spanish (MEX)' },
  { code: 'es_ES', name: 'Spanish (SPA)' },
  { code: 'sw', name: 'Swahili' },
  { code: 'sv', name: 'Swedish' },
  { code: 'ta', name: 'Tamil' },
  { code: 'te', name: 'Telugu' },
  { code: 'th', name: 'Thai' },
  { code: 'tr', name: 'Turkish' },
  { code: 'uk', name: 'Ukrainian' },
  { code: 'ur', name: 'Urdu' },
  { code: 'uz', name: 'Uzbek' },
  { code: 'vi', name: 'Vietnamese' },
  { code: 'zu', name: 'Zulu' },
]

function getLanguageName(code: string): string {
  return languages.find(l => l.code === code)?.name || code
}

// Refetch data when organization changes
watch(() => organizationsStore.selectedOrgId, async () => {
  await fetchAccounts()
  await refreshTemplatesData()
})

onMounted(async () => {
  await fetchAccounts()
  await refreshTemplatesData()
})

async function fetchAccounts() {
  try {
    const response = await api.get('/accounts')
    accounts.value = response.data.data?.accounts || []
    // Validate stored account still exists, fallback to 'all' if not
    if (selectedAccount.value !== 'all' && !accounts.value.some(a => a.name === selectedAccount.value)) {
      selectedAccount.value = 'all'
      localStorage.setItem('templates_selected_account', 'all')
    }
  } catch (error) {
    console.error('Failed to fetch accounts:', error)
  }
}

function onAccountChange(value: string | number | bigint | Record<string, any> | null) {
  if (typeof value !== 'string') return
  selectedAccount.value = value
  localStorage.setItem('templates_selected_account', value)
  currentPage.value = 1
  refreshTemplatesData()
}

function onStatusChange(value: string | number | bigint | Record<string, any> | null) {
  if (typeof value !== 'string') return
  selectedStatus.value = value
  localStorage.setItem('templates_selected_status', value)
  currentPage.value = 1
  refreshTemplatesData()
}

function onCategoryChange(value: string | number | bigint | Record<string, any> | null) {
  if (typeof value !== 'string') return
  selectedCategory.value = value
  localStorage.setItem('templates_selected_category', value)
  currentPage.value = 1
  refreshTemplatesData()
}

function buildTemplateListParams(options?: {
  statusOverride?: string
  categoryOverride?: string
  page?: number
  limit?: number
}) {
  const resolvedStatus = options?.statusOverride ?? selectedStatus.value
  const resolvedCategory = options?.categoryOverride ?? selectedCategory.value
  return {
    account: selectedAccount.value !== 'all' ? selectedAccount.value : undefined,
    status: resolvedStatus !== 'all' ? resolvedStatus : undefined,
    category: resolvedCategory !== 'all' ? resolvedCategory : undefined,
    search: searchQuery.value || undefined,
    page: options?.page ?? currentPage.value,
    limit: options?.limit ?? pageSize,
  }
}

async function fetchCounts(
  keys: string[],
  previous: Record<string, number>,
  override: (key: string) => { statusOverride?: string; categoryOverride?: string },
): Promise<Record<string, number>> {
  const results = await Promise.allSettled(
    keys.map(async (key) => {
      const response = await templatesService.list(buildTemplateListParams({ ...override(key), page: 1, limit: 1 }))
      const data = (response.data as any).data || response.data
      return typeof data.total === 'number' ? data.total : 0
    })
  )
  const next: Record<string, number> = { all: 0 }
  results.forEach((result, index) => {
    const key = keys[index]
    next[key] = result.status === 'fulfilled' ? result.value : (previous[key] ?? 0)
  })
  return next
}

async function fetchTemplateStatusCounts() {
  templateStatusCounts.value = await fetchCounts(statusTabs, templateStatusCounts.value, status => ({ statusOverride: status }))
}

async function fetchTemplateCategoryCounts() {
  templateCategoryCounts.value = await fetchCounts(categoryTabs, templateCategoryCounts.value, category => ({ categoryOverride: category }))
}

async function refreshTemplatesData() {
  await Promise.all([
    fetchTemplates(),
    fetchTemplateStatusCounts(),
    fetchTemplateCategoryCounts(),
  ])
}

const { isRefreshing: isRefreshingView, refreshNow } = useViewRefresh(refreshTemplatesData)

// Keep the tab counts in step with the (debounced) search box.
const debouncedCountsRefresh = useDebounceFn(() => {
  fetchTemplateStatusCounts()
  fetchTemplateCategoryCounts()
}, 300)
watch(searchQuery, () => debouncedCountsRefresh())

function formatStatusLabel(status: string) {
  if (status === 'all') return 'All'
  return status
    .toLowerCase()
    .split('_')
    .map(part => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

function formatCategoryLabel(category: string) {
  if (category === 'all') return 'All'
  return categories.find(item => item.value === category)?.label || category
}

function createCampaignFromTemplate(template: Template) {
  openCampaignFromTemplate(router, template)
}

async function fetchTemplates() {
  isLoading.value = true
  error.value = null
  try {
    const response = await templatesService.list(buildTemplateListParams())
    const data = (response.data as any).data || response.data
    templates.value = data.templates || []
    totalItems.value = data.total ?? templates.value.length
  } catch (err: any) {
    console.error('Failed to fetch templates:', err)
    error.value = t('templates.errorLoadingTemplates')
    toast.error(t('common.failedLoad', { resource: t('resources.templates') }))
    templates.value = []
  } finally {
    isLoading.value = false
  }
}

async function syncTemplates() {
  if (!selectedAccount.value || selectedAccount.value === 'all') {
    toast.error(t('templates.selectAccountFirst'))
    return
  }

  isSyncing.value = true
  try {
    const response = await api.post('/templates/sync', {
      whatsapp_account: selectedAccount.value
    })
    toast.success(response.data.data.message || t('templates.syncSuccess'))
    await refreshTemplatesData()
  } catch (error) {
    toast.error(getErrorMessage(error, t('templates.syncFailed')))
  } finally {
    isSyncing.value = false
  }
}

function openDeleteDialog(template: Template) {
  templateToDelete.value = template
  deleteDialogOpen.value = true
}

async function confirmDeleteTemplate() {
  if (!templateToDelete.value) return

  isDeleting.value = true
  try {
    await api.delete(`/templates/${templateToDelete.value.id}`)
    toast.success(t('common.deletedSuccess', { resource: t('resources.Template') }))
    deleteDialogOpen.value = false
    templateToDelete.value = null
    await refreshTemplatesData()
  } catch (error) {
    toast.error(getErrorMessage(error, t('common.failedDelete', { resource: t('resources.template') })))
  } finally {
    isDeleting.value = false
  }
}

// Dark-first: default is dark mode, light: prefix for light mode
function getStatusBadgeClass(status: string) {
  switch (status) {
    case 'APPROVED':
      return 'bg-green-900 text-green-300 light:bg-green-100 light:text-green-800'
    case 'PENDING':
      return 'bg-yellow-900 text-yellow-300 light:bg-yellow-100 light:text-yellow-800'
    case 'REJECTED':
      return 'bg-red-900 text-red-300 light:bg-red-100 light:text-red-800'
    case 'FLAGGED':
      return 'bg-red-950 text-red-200 light:bg-red-100 light:text-red-900'
    case 'DRAFT':
      return 'bg-gray-800 text-gray-300 light:bg-gray-100 light:text-gray-800'
    case 'DISABLED':
    case 'DELETED':
      return 'bg-slate-800 text-slate-300 light:bg-slate-100 light:text-slate-800'
    case 'PENDING_DELETION':
      return 'bg-orange-900 text-orange-300 light:bg-orange-100 light:text-orange-800'
    case 'REINSTATED':
      return 'bg-emerald-900 text-emerald-300 light:bg-emerald-100 light:text-emerald-800'
    default:
      return 'bg-gray-800 text-gray-300 light:bg-gray-100 light:text-gray-800'
  }
}

function getCategoryBadgeClass(category: string) {
  switch (category) {
    case 'UTILITY':
      return 'bg-blue-900 text-blue-300 light:bg-blue-100 light:text-blue-800'
    case 'MARKETING':
      return 'bg-purple-900 text-purple-300 light:bg-purple-100 light:text-purple-800'
    case 'AUTHENTICATION':
      return 'bg-orange-900 text-orange-300 light:bg-orange-100 light:text-orange-800'
    default:
      return 'bg-gray-800 text-gray-300 light:bg-gray-100 light:text-gray-800'
  }
}

function getHeaderIcon(type: string) {
  switch (type) {
    case 'IMAGE':
      return Image
    case 'VIDEO':
      return Video
    case 'DOCUMENT':
      return FileIcon
    default:
      return MessageSquare
  }
}
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="$t('templates.title')" :subtitle="$t('templates.subtitle')" :icon="FileText" icon-gradient="bg-gradient-to-br from-blue-500 to-cyan-600 shadow-blue-500/20">
      <template #actions>
        <RefreshButton :refreshing="isRefreshingView" :label="$t('common.refresh')" @refresh="refreshNow(true)" />
        <Button variant="outline" size="sm" @click="syncTemplates" :disabled="isSyncing || !selectedAccount || selectedAccount === 'all'">
          <Loader2 v-if="isSyncing" class="h-4 w-4 mr-2 animate-spin" />
          <RefreshCw v-else class="h-4 w-4 mr-2" />
          {{ $t('templates.syncFromMeta') }}
        </Button>
        <RouterLink to="/templates/new">
          <Button variant="outline" size="sm">
            <Plus class="h-4 w-4 mr-2" />
            {{ $t('templates.createTemplate') }}
          </Button>
        </RouterLink>
      </template>
    </PageHeader>

    <ScrollArea class="flex-1">
      <div class="p-6">
        <div>
          <ErrorState
            v-if="error && !isLoading"
            :title="$t('common.loadErrorTitle')"
            :description="error"
            :retry-label="$t('common.retry')"
            @retry="fetchTemplates"
          />
          <Card v-else>
            <CardHeader>
              <div class="flex flex-col gap-4">
                <div class="flex items-center justify-between flex-wrap gap-4">
                  <div>
                    <CardTitle>{{ $t('templates.yourTemplates') }}</CardTitle>
                    <CardDescription>{{ $t('templates.yourTemplatesDesc') }}</CardDescription>
                  </div>
                  <div class="flex w-full flex-col gap-4 md:w-auto md:flex-row md:items-center">
                    <div class="flex items-center gap-2">
                      <Label class="text-sm text-muted-foreground">{{ $t('templates.account') }}:</Label>
                      <Select v-model="selectedAccount" @update:model-value="onAccountChange">
                        <SelectTrigger class="w-full sm:w-[180px]">
                          <SelectValue :placeholder="$t('templates.allAccounts')" />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="all">{{ $t('templates.allAccounts') }}</SelectItem>
                          <SelectItem v-for="account in accounts" :key="account.id" :value="account.name">
                            {{ account.name }}
                          </SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    <SearchInput v-model="searchQuery" :placeholder="$t('templates.searchTemplates') + '...'" class="w-full md:w-64" />
                  </div>
                </div>
                <div>
                  <Label class="mb-2 block text-sm text-muted-foreground">{{ $t('templates.status') }}:</Label>
                  <Tabs :model-value="selectedStatus" @update:model-value="onStatusChange" class="w-full">
                    <TabsList class="h-auto w-full justify-start overflow-x-auto rounded-lg border border-white/[0.08] bg-white/[0.03] p-1 light:border-gray-200 light:bg-gray-100">
                      <TabsTrigger
                        v-for="status in statusTabs"
                        :key="status"
                        :value="status"
                        class="h-auto min-w-fit gap-2 px-3 py-2 text-xs whitespace-nowrap"
                      >
                        <span>{{ formatStatusLabel(status) }}</span>
                        <span class="rounded-full bg-black/20 px-2 py-0.5 text-[11px] light:bg-white">
                          {{ templateStatusCounts[status] ?? 0 }}
                        </span>
                      </TabsTrigger>
                    </TabsList>
                  </Tabs>
                </div>
                <div>
                  <Label class="mb-2 block text-sm text-muted-foreground">{{ $t('templates.category') }}:</Label>
                  <Tabs :model-value="selectedCategory" @update:model-value="onCategoryChange" class="w-full">
                    <TabsList class="h-auto w-full justify-start overflow-x-auto rounded-lg border border-white/[0.08] bg-white/[0.03] p-1 light:border-gray-200 light:bg-gray-100">
                      <TabsTrigger
                        v-for="category in categoryTabs"
                        :key="category"
                        :value="category"
                        class="h-auto min-w-fit gap-2 px-3 py-2 text-xs whitespace-nowrap"
                      >
                        <span>{{ formatCategoryLabel(category) }}</span>
                        <span class="rounded-full bg-black/20 px-2 py-0.5 text-[11px] light:bg-white">
                          {{ templateCategoryCounts[category] ?? 0 }}
                        </span>
                      </TabsTrigger>
                    </TabsList>
                  </Tabs>
                </div>
              </div>
            </CardHeader>
            <CardContent>
              <DataTable
                :items="templates"
                :columns="columns"
                :is-loading="isLoading"
                :empty-icon="FileText"
                :empty-title="$t('templates.noTemplatesFound')"
                :empty-description="$t('templates.noTemplatesFoundDesc')"
                server-pagination
                :current-page="currentPage"
                :total-items="totalItems"
                :page-size="pageSize"
                item-name="templates"
                @page-change="handlePageChange"
                v-model:sort-key="sortKey"
                v-model:sort-direction="sortDirection"
              >
                <template #cell-name="{ item: template }">
                  <RouterLink :to="`/templates/${template.id}`" class="text-inherit no-underline hover:opacity-80">
                    <span class="font-medium">{{ template.display_name || template.name }}</span>
                    <p class="text-xs font-mono text-muted-foreground">{{ template.name }}</p>
                  </RouterLink>
                </template>
                <template #cell-category="{ item: template }">
                  <Badge :class="getCategoryBadgeClass(template.category)" class="text-xs">
                    {{ template.category }}
                  </Badge>
                </template>
                <template #cell-status="{ item: template }">
                  <Badge :class="getStatusBadgeClass(template.status)" class="text-xs">
                    {{ template.status }}
                  </Badge>
                </template>
                <template #cell-language="{ item: template }">
                  <span class="text-muted-foreground">{{ getLanguageName(template.language) }}</span>
                </template>
                <template #cell-header_type="{ item: template }">
                  <div class="flex items-center gap-1">
                    <component :is="getHeaderIcon(template.header_type)" class="h-4 w-4 text-muted-foreground" />
                    <span class="text-muted-foreground text-sm">{{ template.header_type || 'NONE' }}</span>
                  </div>
                </template>
                <template #cell-actions="{ item: template }">
                  <div class="flex items-center justify-end gap-1">
                    <IconButton
                      v-if="canCreateCampaign"
                      :icon="Megaphone"
                      :label="$t('campaigns.createCampaign')"
                      class="h-8 w-8 text-orange-600 hover:text-orange-700"
                      @click="createCampaignFromTemplate(template)"
                    />
                    <RouterLink :to="`/templates/${template.id}`">
                      <IconButton
                        :icon="Pencil"
                        :label="$t('common.edit')"
                        class="h-8 w-8"
                      />
                    </RouterLink>
                    <IconButton
                      :icon="Trash2"
                      :label="$t('common.delete')"
                      class="h-8 w-8 text-destructive"
                      @click="openDeleteDialog(template)"
                    />
                  </div>
                </template>
                <template #empty-action>
                  <div class="flex items-center justify-center gap-2">
                    <Button variant="outline" size="sm" @click="syncTemplates" :disabled="!selectedAccount || selectedAccount === 'all'">
                      <RefreshCw class="h-4 w-4 mr-2" />
                      {{ $t('templates.syncFromMeta') }}
                    </Button>
                    <RouterLink to="/templates/new">
                      <Button variant="outline" size="sm">
                        <Plus class="h-4 w-4 mr-2" />
                        {{ $t('templates.createTemplate') }}
                      </Button>
                    </RouterLink>
                  </div>
                </template>
              </DataTable>
            </CardContent>
          </Card>
        </div>
      </div>
    </ScrollArea>

    <!-- Delete Confirmation Dialog -->
    <DeleteConfirmDialog
      v-model:open="deleteDialogOpen"
      :title="$t('templates.deleteTemplate')"
      :item-name="templateToDelete?.display_name || templateToDelete?.name"
      :is-submitting="isDeleting"
      @confirm="confirmDeleteTemplate"
    />
  </div>
</template>
