<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { PageHeader, DeleteConfirmDialog, ErrorState, RefreshButton } from '@/components/shared'
import CatalogProductsCard from '@/components/catalogs/CatalogProductsCard.vue'
import ProductFormDialog from '@/components/catalogs/ProductFormDialog.vue'
import { catalogsService, type Catalog, type CatalogProduct } from '@/services/api'
import { useAuthStore } from '@/stores/auth'
import { useViewRefresh } from '@/composables/useViewRefresh'
import { getErrorMessage } from '@/lib/api-utils'
import { formatDate, formatDateTime } from '@/lib/utils'
import { ShoppingBag, Trash2 } from 'lucide-vue-next'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()

const canWrite = computed(() => authStore.hasPermission('catalogs', 'write'))
const canDelete = computed(() => authStore.hasPermission('catalogs', 'delete'))

const catalogId = computed(() => String(route.params.id))
const catalog = ref<Catalog | null>(null)
const products = ref<CatalogProduct[]>([])
const isLoading = ref(false)
const error = ref(false)

const isProductDialogOpen = ref(false)
const editingProduct = ref<CatalogProduct | null>(null)
const productToDelete = ref<CatalogProduct | null>(null)
const isDeleteProductOpen = ref(false)
const isDeleteCatalogOpen = ref(false)
const isDeleting = ref(false)

const breadcrumbs = computed(() => [{ label: t('nav.catalogs'), href: '/catalogs' }, { label: catalog.value?.name || t('resources.Catalog') }])

async function fetchCatalog() {
  isLoading.value = true
  error.value = false
  try {
    const response = await catalogsService.get(catalogId.value)
    const data = ((response.data as any).data || response.data) as Catalog
    catalog.value = data
    products.value = data.products || []
  } catch (e) {
    error.value = true
    toast.error(getErrorMessage(e, t('common.failedLoad', { resource: t('resources.catalog') })))
  } finally {
    isLoading.value = false
  }
}

const { isRefreshing, refreshNow } = useViewRefresh(fetchCatalog)

function openAdd() { editingProduct.value = null; isProductDialogOpen.value = true }
function openEdit(product: CatalogProduct) { editingProduct.value = product; isProductDialogOpen.value = true }
function askDeleteProduct(product: CatalogProduct) { productToDelete.value = product; isDeleteProductOpen.value = true }

async function confirmDeleteProduct() {
  if (!productToDelete.value) return
  isDeleting.value = true
  try {
    await catalogsService.deleteProduct(productToDelete.value.id)
    toast.success(t('common.deletedSuccess', { resource: t('resources.Product') }))
    isDeleteProductOpen.value = false
    productToDelete.value = null
    await fetchCatalog()
  } catch (e) {
    toast.error(getErrorMessage(e, t('common.failedDelete', { resource: t('resources.product') })))
  } finally {
    isDeleting.value = false
  }
}

async function confirmDeleteCatalog() {
  if (!catalog.value) return
  isDeleting.value = true
  try {
    await catalogsService.delete(catalog.value.id)
    toast.success(t('common.deletedSuccess', { resource: t('resources.Catalog') }))
    isDeleteCatalogOpen.value = false
    router.push('/catalogs')
  } catch (e) {
    toast.error(getErrorMessage(e, t('common.failedDelete', { resource: t('resources.catalog') })))
  } finally {
    isDeleting.value = false
  }
}

watch(catalogId, () => fetchCatalog())
onMounted(() => fetchCatalog())
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="catalog?.name || $t('catalogs.title')" :subtitle="$t('catalogs.detailSubtitle')" :icon="ShoppingBag" icon-gradient="bg-gradient-to-br from-orange-500 to-rose-600 shadow-orange-500/20" back-link="/catalogs" :breadcrumbs="breadcrumbs">
      <template #actions>
        <RefreshButton :refreshing="isRefreshing" :label="$t('common.refresh')" @refresh="refreshNow(true)" />
        <Button v-if="canDelete && catalog" variant="outline" size="sm" class="text-destructive" @click="isDeleteCatalogOpen = true"><Trash2 class="h-4 w-4 mr-2" />{{ $t('catalogs.deleteCatalog') }}</Button>
      </template>
    </PageHeader>

    <ErrorState v-if="error && !isLoading" :title="$t('common.loadErrorTitle')" :description="$t('catalogs.detailLoadError')" :retry-label="$t('common.retry')" class="flex-1" @retry="fetchCatalog" />

    <ScrollArea v-else class="flex-1">
      <div class="p-6 space-y-6">
        <Card v-if="catalog">
          <CardContent class="pt-6">
            <dl class="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
              <div><dt class="text-muted-foreground">{{ $t('catalogs.whatsappAccount') }}</dt><dd class="font-medium mt-1">{{ catalog.whatsapp_account }}</dd></div>
              <div><dt class="text-muted-foreground">{{ $t('catalogs.metaId') }}</dt><dd class="font-mono text-xs mt-1 break-all">{{ catalog.meta_catalog_id }}</dd></div>
              <div><dt class="text-muted-foreground">{{ $t('catalogs.products') }}</dt><dd class="font-medium mt-1">{{ products.length }}</dd></div>
              <div><dt class="text-muted-foreground">{{ $t('catalogs.created') }}</dt><dd class="font-medium mt-1" :title="formatDateTime(catalog.created_at)">{{ formatDate(catalog.created_at) }}</dd></div>
            </dl>
          </CardContent>
        </Card>

        <CatalogProductsCard :products="products" :is-loading="isLoading" :can-write="canWrite" :can-delete="canDelete" @add="openAdd" @edit="openEdit" @delete="askDeleteProduct" />
      </div>
    </ScrollArea>

    <ProductFormDialog v-model:open="isProductDialogOpen" :catalog-id="catalogId" :product="editingProduct" @saved="fetchCatalog" />
    <DeleteConfirmDialog v-model:open="isDeleteProductOpen" :title="$t('catalogs.deleteProduct')" :item-name="productToDelete?.name" :is-submitting="isDeleting" @confirm="confirmDeleteProduct">
      <p class="text-sm text-muted-foreground">{{ $t('catalogs.deleteProductWarning') }}</p>
    </DeleteConfirmDialog>
    <DeleteConfirmDialog v-model:open="isDeleteCatalogOpen" :title="$t('catalogs.deleteCatalog')" :item-name="catalog?.name" :is-submitting="isDeleting" @confirm="confirmDeleteCatalog">
      <p class="text-sm text-muted-foreground">{{ $t('catalogs.deleteCatalogWarning') }}</p>
    </DeleteConfirmDialog>
  </div>
</template>
