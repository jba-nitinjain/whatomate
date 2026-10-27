<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { CrudFormDialog } from '@/components/shared'
import { catalogsService, type CatalogProduct, type CatalogProductPayload } from '@/services/api'
import { getErrorMessage } from '@/lib/api-utils'
import MetaErrorAlert from './MetaErrorAlert.vue'
import { CATALOG_CURRENCIES, DEFAULT_CATALOG_CURRENCY, fromMinorUnits, isHttpUrl, toMinorUnits } from './catalogUtils'

const props = defineProps<{
  catalogId: string
  product: CatalogProduct | null
}>()
const emit = defineEmits<{ saved: [] }>()
const open = defineModel<boolean>('open', { default: false })

const { t } = useI18n()
const isEditing = computed(() => !!props.product)
const form = ref({ name: '', description: '', price: '', currency: DEFAULT_CATALOG_CURRENCY, url: '', image_url: '', retailer_id: '' })
const isSubmitting = ref(false)
const errorMessage = ref<string | null>(null)

const currencies = computed(() => {
  const list: string[] = [...CATALOG_CURRENCIES]
  if (form.value.currency && !list.includes(form.value.currency)) list.unshift(form.value.currency)
  return list
})

watch(open, (isOpen) => {
  if (!isOpen) return
  errorMessage.value = null
  const p = props.product
  form.value = p
    ? { name: p.name, description: p.description, price: fromMinorUnits(p.price), currency: p.currency || DEFAULT_CATALOG_CURRENCY, url: p.url, image_url: p.image_url, retailer_id: p.retailer_id }
    : { name: '', description: '', price: '', currency: DEFAULT_CATALOG_CURRENCY, url: '', image_url: '', retailer_id: '' }
})

function validate(): string | null {
  const f = form.value
  if (!f.name.trim()) return t('catalogs.validationName')
  if (toMinorUnits(f.price) <= 0) return t('catalogs.validationPrice')
  if (!f.image_url.trim() || !f.url.trim() || !f.retailer_id.trim()) return t('catalogs.validationRequired')
  if (!isHttpUrl(f.image_url.trim()) || !isHttpUrl(f.url.trim())) return t('catalogs.invalidUrl')
  return null
}

async function submit() {
  const invalid = validate()
  if (invalid) { toast.error(invalid); return }
  const f = form.value
  const payload: CatalogProductPayload = {
    name: f.name.trim(),
    description: f.description.trim(),
    price: toMinorUnits(f.price),
    currency: f.currency,
    url: f.url.trim(),
    image_url: f.image_url.trim(),
    retailer_id: f.retailer_id.trim(),
  }
  isSubmitting.value = true
  errorMessage.value = null
  try {
    if (props.product) {
      await catalogsService.updateProduct(props.product.id, payload)
      toast.success(t('common.updatedSuccess', { resource: t('resources.Product') }))
    } else {
      await catalogsService.createProduct(props.catalogId, payload)
      toast.success(t('common.createdSuccess', { resource: t('resources.Product') }))
    }
    open.value = false
    emit('saved')
  } catch (err) {
    const key = props.product ? 'common.failedUpdate' : 'common.failedCreate'
    errorMessage.value = getErrorMessage(err, t(key, { resource: t('resources.product') }))
    toast.error(errorMessage.value)
  } finally {
    isSubmitting.value = false
  }
}
</script>

<template>
  <CrudFormDialog
    v-model:open="open"
    :is-editing="isEditing"
    :is-submitting="isSubmitting"
    :edit-title="$t('catalogs.editProduct')"
    :create-title="$t('catalogs.addProduct')"
    :edit-description="$t('catalogs.editProductDesc')"
    :create-description="$t('catalogs.addProductDesc')"
    :create-submit-label="$t('common.create')"
    :edit-submit-label="$t('common.save')"
    :cancel-label="$t('common.cancel')"
    max-width="max-w-lg"
    @submit="submit"
  >
    <div class="space-y-4">
      <div class="space-y-2">
        <Label>{{ $t('catalogs.productName') }} <span class="text-destructive">*</span></Label>
        <Input v-model="form.name" :placeholder="$t('catalogs.productNamePlaceholder')" maxlength="255" />
      </div>
      <div class="grid grid-cols-2 gap-3">
        <div class="space-y-2">
          <Label>{{ $t('catalogs.price') }} <span class="text-destructive">*</span></Label>
          <Input v-model="form.price" type="number" min="0" step="0.01" inputmode="decimal" placeholder="0.00" />
        </div>
        <div class="space-y-2">
          <Label>{{ $t('catalogs.currency') }}</Label>
          <Select v-model="form.currency">
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem v-for="c in currencies" :key="c" :value="c">{{ c }}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>
      <div class="space-y-2">
        <Label>{{ $t('catalogs.retailerId') }} <span class="text-destructive">*</span></Label>
        <Input v-model="form.retailer_id" :placeholder="$t('catalogs.retailerIdPlaceholder')" maxlength="100" :disabled="isEditing" />
        <p class="text-xs text-muted-foreground">{{ isEditing ? $t('catalogs.retailerIdLocked') : $t('catalogs.retailerIdHint') }}</p>
      </div>
      <div class="space-y-2">
        <Label>{{ $t('catalogs.imageUrl') }} <span class="text-destructive">*</span></Label>
        <Input v-model="form.image_url" type="url" placeholder="https://example.com/image.jpg" maxlength="500" />
        <p class="text-xs text-muted-foreground">{{ $t('catalogs.imageUrlHint') }}</p>
      </div>
      <div class="space-y-2">
        <Label>{{ $t('catalogs.productUrl') }} <span class="text-destructive">*</span></Label>
        <Input v-model="form.url" type="url" placeholder="https://example.com/product" maxlength="500" />
      </div>
      <div class="space-y-2">
        <Label>{{ $t('catalogs.description') }}</Label>
        <Textarea v-model="form.description" :placeholder="$t('catalogs.descriptionPlaceholder')" :rows="3" />
      </div>
      <MetaErrorAlert :message="errorMessage" />
    </div>
  </CrudFormDialog>
</template>
