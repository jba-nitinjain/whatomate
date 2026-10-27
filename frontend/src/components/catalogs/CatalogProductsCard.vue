<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { SearchInput, DataTable, IconButton, type Column } from '@/components/shared'
import type { CatalogProduct } from '@/services/api'
import { Plus, Package, Pencil, Trash2, ExternalLink } from 'lucide-vue-next'
import { formatCatalogPrice } from './catalogUtils'

const props = defineProps<{
  products: CatalogProduct[]
  isLoading: boolean
  canWrite: boolean
  canDelete: boolean
}>()
const emit = defineEmits<{ add: []; edit: [product: CatalogProduct]; delete: [product: CatalogProduct] }>()

const { t } = useI18n()
const searchQuery = ref('')
const sortKey = ref('name')
const sortDirection = ref<'asc' | 'desc'>('asc')

const columns = computed<Column<CatalogProduct>[]>(() => [
  { key: 'name', label: t('catalogs.productName'), sortable: true },
  { key: 'retailer_id', label: t('catalogs.retailerId'), sortable: true },
  { key: 'price', label: t('catalogs.price'), sortable: true },
  { key: 'status', label: t('catalogs.status'), sortable: true, sortKey: 'is_active' },
  { key: 'actions', label: t('common.actions'), align: 'right' },
])

const filtered = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return props.products
  return props.products.filter(p => p.name.toLowerCase().includes(q) || (p.retailer_id || '').toLowerCase().includes(q))
})
</script>

<template>
  <Card>
    <CardHeader>
      <div class="flex items-center justify-between flex-wrap gap-4">
        <div>
          <CardTitle>{{ $t('catalogs.productsTitle') }}</CardTitle>
          <CardDescription>{{ $t('catalogs.productsDesc') }}</CardDescription>
        </div>
        <div class="flex items-center gap-2">
          <SearchInput v-model="searchQuery" :placeholder="$t('catalogs.searchProducts') + '...'" class="w-64" />
          <Button v-if="canWrite" variant="outline" size="sm" @click="emit('add')"><Plus class="h-4 w-4 mr-2" />{{ $t('catalogs.addProduct') }}</Button>
        </div>
      </div>
    </CardHeader>
    <CardContent>
      <DataTable :items="filtered" :columns="columns" :is-loading="isLoading" :empty-icon="Package" :empty-title="searchQuery ? $t('catalogs.noMatchingProducts') : $t('catalogs.noProducts')" :empty-description="searchQuery ? $t('catalogs.noMatchingDesc') : $t('catalogs.noProductsDesc')" v-model:sort-key="sortKey" v-model:sort-direction="sortDirection" item-name="products">
        <template #cell-name="{ item }">
          <div class="flex items-center gap-3">
            <img v-if="item.image_url" :src="item.image_url" :alt="item.name" class="h-10 w-10 rounded object-cover border border-white/[0.08] light:border-gray-200" loading="lazy" referrerpolicy="no-referrer" />
            <div v-else class="h-10 w-10 rounded bg-muted flex items-center justify-center"><Package class="h-4 w-4 text-muted-foreground" /></div>
            <div class="min-w-0">
              <p class="font-medium truncate max-w-[260px]">{{ item.name }}</p>
              <p v-if="item.description" class="text-xs text-muted-foreground truncate max-w-[260px]">{{ item.description }}</p>
            </div>
          </div>
        </template>
        <template #cell-retailer_id="{ item }"><span class="font-mono text-xs text-muted-foreground">{{ item.retailer_id || '—' }}</span></template>
        <template #cell-price="{ item }"><span class="font-medium">{{ formatCatalogPrice(item.price, item.currency) }}</span></template>
        <template #cell-status="{ item }">
          <Badge v-if="item.is_active" class="bg-emerald-500/20 text-emerald-400 border-transparent text-xs">{{ $t('common.active') }}</Badge>
          <Badge v-else variant="secondary" class="text-xs">{{ $t('common.inactive') }}</Badge>
        </template>
        <template #cell-actions="{ item }">
          <div class="flex items-center justify-end gap-1">
            <a v-if="item.url" :href="item.url" target="_blank" rel="noopener noreferrer"><IconButton :icon="ExternalLink" :label="$t('catalogs.openProductPage')" class="h-8 w-8" /></a>
            <IconButton v-if="canWrite" :icon="Pencil" :label="$t('catalogs.editProduct')" class="h-8 w-8" @click="emit('edit', item)" />
            <IconButton v-if="canDelete" :icon="Trash2" :label="$t('catalogs.deleteProduct')" class="h-8 w-8 text-destructive" @click="emit('delete', item)" />
          </div>
        </template>
        <template #empty-action>
          <Button v-if="canWrite && !searchQuery" variant="outline" size="sm" @click="emit('add')"><Plus class="h-4 w-4 mr-2" />{{ $t('catalogs.addProduct') }}</Button>
        </template>
      </DataTable>
    </CardContent>
  </Card>
</template>
