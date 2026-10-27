<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { CrudFormDialog } from '@/components/shared'
import { catalogsService, type CatalogSyncResult } from '@/services/api'
import { getErrorMessage } from '@/lib/api-utils'
import MetaErrorAlert from './MetaErrorAlert.vue'
import type { CatalogAccountOption } from './catalogUtils'

const props = defineProps<{
  accounts: CatalogAccountOption[]
  defaultAccount?: string
}>()
const emit = defineEmits<{ synced: [account: string] }>()
const open = defineModel<boolean>('open', { default: false })

const { t } = useI18n()
const account = ref('')
const isSubmitting = ref(false)
const errorMessage = ref<string | null>(null)

watch(open, (isOpen) => {
  if (!isOpen) return
  errorMessage.value = null
  account.value = props.defaultAccount || (props.accounts.length === 1 ? props.accounts[0].name : '')
})

async function submit() {
  if (!account.value) { toast.error(t('catalogs.selectAccountFirst')); return }
  isSubmitting.value = true
  errorMessage.value = null
  try {
    const response = await catalogsService.sync(account.value)
    const result = ((response.data as any).data || response.data) as CatalogSyncResult
    toast.success(t('catalogs.syncResult', { synced: result.synced ?? 0, total: result.total ?? 0 }))
    open.value = false
    emit('synced', account.value)
  } catch (err) {
    errorMessage.value = getErrorMessage(err, t('catalogs.syncFailed'))
    toast.error(errorMessage.value)
  } finally {
    isSubmitting.value = false
  }
}
</script>

<template>
  <CrudFormDialog
    v-model:open="open"
    :is-submitting="isSubmitting"
    :title="$t('catalogs.syncTitle')"
    :description="$t('catalogs.syncDesc')"
    :submit-label="$t('catalogs.syncNow')"
    :cancel-label="$t('common.cancel')"
    @submit="submit"
  >
    <div class="space-y-4">
      <div class="space-y-2">
        <Label>{{ $t('catalogs.whatsappAccount') }} <span class="text-destructive">*</span></Label>
        <Select v-model="account">
          <SelectTrigger data-testid="catalog-sync-account-select"><SelectValue :placeholder="$t('catalogs.selectAccount')" /></SelectTrigger>
          <SelectContent>
            <SelectItem v-for="acc in accounts" :key="acc.id" :value="acc.name">{{ acc.name }}</SelectItem>
          </SelectContent>
        </Select>
        <p v-if="accounts.length === 0" class="text-xs text-muted-foreground">{{ $t('catalogs.noAccounts') }}</p>
      </div>
      <MetaErrorAlert :message="errorMessage" />
    </div>
  </CrudFormDialog>
</template>
