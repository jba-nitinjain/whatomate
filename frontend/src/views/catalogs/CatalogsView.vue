<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { PageHeader, SearchInput, DeleteConfirmDialog, DataTable, IconButton, ErrorState, RefreshButton, type Column } from '@/components/shared'
import CatalogFormDialog from '@/components/catalogs/CatalogFormDialog.vue'
import CatalogSyncDialog from '@/components/catalogs/CatalogSyncDialog.vue'
import { extractAccounts, type CatalogAccountOption } from '@/components/catalogs/catalogUtils'
import { accountsService, catalogsService, type Catalog } from '@/services/api'
import { useAuthStore } from '@/stores/auth'
import { useOrganizationsStore } from '@/stores/organizations'
import { useViewRefresh } from '@/composables/useViewRefresh'
import { getErrorMessage } from '@/lib/api-utils'
import { formatDate } from '@/lib/utils'
import { Plus, ShoppingBag, Eye, Trash2, RefreshCw } from 'lucide-vue-next'

const ACCOUNT_KEY = 'catalogs_selected_account'
const { t } = useI18n()
const authStore = useAuthStore()
const organizationsStore = useOrganizationsStore()

const canWrite = computed(() => authStore.hasPermission('catalogs', 'write'))
const canDelete = computed(() => authStore.hasPermission('catalogs', 'delete'))
const canSync = computed(() => authStore.hasPermission('catalogs', 'sync'))

const catalogs = ref<Catalog[]>([])
const accounts = ref<CatalogAccountOption[]>([])
const isLoading = ref(false)
const error = ref(false)
const searchQuery = ref('')
const selectedAccount = ref(readStoredAccount())
const isCreateOpen = ref(false)
const isSyncOpen = ref(false)
const isDeleteOpen = ref(false)
const isDeleting = ref(false)
const catalogToDelete = ref<Catalog | null>(null)
const sortKey = ref('name')
const sortDirection = ref<'asc' | 'desc'>('asc')

const accountFilter = computed(() => (selectedAccount.value !== 'all' ? selectedAccount.value : undefined))

const columns = computed<Column<Catalog>[]>(() => [
  { key: 'name', label: t('catalogs.name'), sortable: true },
  { key: 'whatsapp_account', label: t('catalogs.whatsappAccount'), sortable: true },
  { key: 'product_count', label: t('catalogs.products'), sortable: true },
  { key: 'meta_catalog_id', label: t('catalogs.metaId') },
  { key: 'created_at', label: t('catalogs.created'), sortable: true },
  { key: 'actions', label: t('common.actions'), align: 'right' },
])

const filteredCatalogs = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return catalogs.value
  return catalogs.value.filter(c => c.name.toLowerCase().includes(q) || c.meta_catalog_id.includes(q))
})

function readStoredAccount(): string {
  try { return localStorage.getItem(ACCOUNT_KEY) || 'all' } catch { return 'all' }
}

async function fetchAccounts() {
  try {
    const response = await accountsService.list()
    accounts.value = extractAccounts(response.data)
    if (selectedAccount.value !== 'all' && !accounts.value.some(a => a.name === selectedAccount.value)) {
      selectedAccount.value = 'all'
    }
  } catch (e) {
    toast.error(getErrorMessage(e, t('common.failedLoad', { resource: t('resources.accounts') })))
  }
}

async function fetchCatalogs() {
  isLoading.value = true
  error.value = false
  try {
    const response = await catalogsService.list({ whatsapp_account: accountFilter.value })
    const data = (response.data as any).data || response.data
    catalogs.value = data.catalogs || []
  } catch (e) {
    error.value = true
    toast.error(getErrorMessage(e, t('common.failedLoad', { resource: t('resources.catalogs') })))
  } finally {
    isLoading.value = false
  }
}

const { isRefreshing, refreshNow } = useViewRefresh(fetchCatalogs)

async function confirmDelete() {
  if (!catalogToDelete.value) return
  isDeleting.value = true
  try {
    await catalogsService.delete(catalogToDelete.value.id)
    toast.success(t('common.deletedSuccess', { resource: t('resources.Catalog') }))
    isDeleteOpen.value = false
    catalogToDelete.value = null
    await fetchCatalogs()
  } catch (e) {
    toast.error(getErrorMessage(e, t('common.failedDelete', { resource: t('resources.catalog') })))
  } finally {
    isDeleting.value = false
  }
}

function askDelete(catalog: Catalog) { catalogToDelete.value = catalog; isDeleteOpen.value = true }

watch(selectedAccount, (value) => {
  try { localStorage.setItem(ACCOUNT_KEY, value) } catch { /* storage unavailable */ }
  fetchCatalogs()
})
watch(() => organizationsStore.selectedOrgId, () => { fetchAccounts(); fetchCatalogs() })
onMounted(() => { fetchAccounts(); fetchCatalogs() })
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="$t('catalogs.title')" :subtitle="$t('catalogs.subtitle')" :icon="ShoppingBag" icon-gradient="bg-gradient-to-br from-orange-500 to-rose-600 shadow-orange-500/20">
      <template #actions>
        <RefreshButton :refreshing="isRefreshing" :label="$t('common.refresh')" @refresh="refreshNow(true)" />
        <Button v-if="canSync" variant="outline" size="sm" @click="isSyncOpen = true"><RefreshCw class="h-4 w-4 mr-2" />{{ $t('catalogs.syncFromMeta') }}</Button>
        <Button v-if="canWrite" variant="outline" size="sm" @click="isCreateOpen = true"><Plus class="h-4 w-4 mr-2" />{{ $t('catalogs.createCatalog') }}</Button>
      </template>
    </PageHeader>

    <ErrorState v-if="error && !isLoading" :title="$t('common.loadErrorTitle')" :description="$t('catalogs.loadError')" :retry-label="$t('common.retry')" class="flex-1" @retry="fetchCatalogs" />

    <ScrollArea v-else class="flex-1">
      <div class="p-6">
        <Card>
          <CardHeader>
            <div class="flex items-center justify-between flex-wrap gap-4">
              <div>
                <CardTitle>{{ $t('catalogs.yourCatalogs') }}</CardTitle>
                <CardDescription>{{ $t('catalogs.yourCatalogsDesc') }}</CardDescription>
              </div>
              <div class="flex items-center gap-2 flex-wrap">
                <Select v-model="selectedAccount">
                  <SelectTrigger class="w-[200px]" data-testid="catalog-account-filter"><SelectValue :placeholder="$t('catalogs.allAccounts')" /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">{{ $t('catalogs.allAccounts') }}</SelectItem>
                    <SelectItem v-for="acc in accounts" :key="acc.id" :value="acc.name">{{ acc.name }}</SelectItem>
                  </SelectContent>
                </Select>
                <SearchInput v-model="searchQuery" :placeholder="$t('catalogs.searchCatalogs') + '...'" class="w-64" />
              </div>
            </div>
          </CardHeader>
          <CardContent>
            <DataTable :items="filteredCatalogs" :columns="columns" :is-loading="isLoading" :empty-icon="ShoppingBag" :empty-title="searchQuery ? $t('catalogs.noMatching') : $t('catalogs.noCatalogs')" :empty-description="searchQuery ? $t('catalogs.noMatchingDesc') : $t('catalogs.noCatalogsDesc')" v-model:sort-key="sortKey" v-model:sort-direction="sortDirection" item-name="catalogs">
              <template #cell-name="{ item }">
                <RouterLink :to="`/catalogs/${item.id}`" class="font-medium text-inherit no-underline hover:opacity-80">{{ item.name }}</RouterLink>
              </template>
              <template #cell-whatsapp_account="{ item }"><Badge variant="outline" class="text-xs">{{ item.whatsapp_account }}</Badge></template>
              <template #cell-product_count="{ item }"><span class="text-muted-foreground">{{ item.product_count }}</span></template>
              <template #cell-meta_catalog_id="{ item }"><span class="font-mono text-xs text-muted-foreground">{{ item.meta_catalog_id }}</span></template>
              <template #cell-created_at="{ item }"><span class="text-muted-foreground">{{ formatDate(item.created_at) }}</span></template>
              <template #cell-actions="{ item }">
                <div class="flex items-center justify-end gap-1">
                  <RouterLink :to="`/catalogs/${item.id}`"><IconButton :icon="Eye" :label="$t('catalogs.viewProducts')" class="h-8 w-8" /></RouterLink>
                  <IconButton v-if="canDelete" :icon="Trash2" :label="$t('catalogs.deleteCatalog')" class="h-8 w-8 text-destructive" @click="askDelete(item)" />
                </div>
              </template>
              <template #empty-action>
                <div class="flex gap-2 justify-center">
                  <Button v-if="canSync" variant="outline" size="sm" @click="isSyncOpen = true"><RefreshCw class="h-4 w-4 mr-2" />{{ $t('catalogs.syncFromMeta') }}</Button>
                  <Button v-if="canWrite" variant="outline" size="sm" @click="isCreateOpen = true"><Plus class="h-4 w-4 mr-2" />{{ $t('catalogs.createCatalog') }}</Button>
                </div>
              </template>
            </DataTable>
          </CardContent>
        </Card>
      </div>
    </ScrollArea>

    <CatalogFormDialog v-model:open="isCreateOpen" :accounts="accounts" :default-account="accountFilter" @created="fetchCatalogs" />
    <CatalogSyncDialog v-model:open="isSyncOpen" :accounts="accounts" :default-account="accountFilter" @synced="fetchCatalogs" />
    <DeleteConfirmDialog v-model:open="isDeleteOpen" :title="$t('catalogs.deleteCatalog')" :item-name="catalogToDelete?.name" :is-submitting="isDeleting" @confirm="confirmDelete">
      <p class="text-sm text-muted-foreground">{{ $t('catalogs.deleteCatalogWarning') }}</p>
    </DeleteConfirmDialog>
  </div>
</template>
