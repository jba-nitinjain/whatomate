<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { CrudFormDialog } from '@/components/shared'
import { organizationsService, type Role } from '@/services/api'
import { getErrorMessage } from '@/lib/api-utils'
import { toast } from 'vue-sonner'
import { pickDefaultRoleId } from './roles'

const props = defineProps<{ roles: Role[] }>()
const open = defineModel<boolean>('open', { default: false })
const emit = defineEmits<{ added: [] }>()

const { t } = useI18n()
const email = ref('')
const roleId = ref('')
const isSubmitting = ref(false)

// Reset the form every time the dialog opens so a previous attempt never leaks in.
watch(open, (isOpen) => {
  if (!isOpen) return
  email.value = ''
  roleId.value = pickDefaultRoleId(props.roles)
})

// Roles may still be loading when the dialog first opens.
watch(() => props.roles, (roles) => {
  if (open.value && !roleId.value) roleId.value = pickDefaultRoleId(roles)
})

async function submit() {
  const value = email.value.trim()
  if (!value) {
    toast.error(t('orgMembers.emailRequired'))
    return
  }
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value)) {
    toast.error(t('validation.email'))
    return
  }
  if (!roleId.value) {
    toast.error(t('orgMembers.roleRequired'))
    return
  }
  isSubmitting.value = true
  try {
    await organizationsService.addMember({ email: value, role_id: roleId.value })
    toast.success(t('orgMembers.memberAdded'))
    open.value = false
    emit('added')
  } catch (e) {
    toast.error(getErrorMessage(e, t('orgMembers.addFailed')))
  } finally {
    isSubmitting.value = false
  }
}
</script>

<template>
  <CrudFormDialog
    v-model:open="open"
    :is-submitting="isSubmitting"
    :create-title="$t('orgMembers.addTitle')"
    :create-description="$t('orgMembers.addDesc')"
    :create-submit-label="$t('orgMembers.addMember')"
    :cancel-label="$t('common.cancel')"
    max-width="max-w-md"
    @submit="submit"
  >
    <div class="space-y-4">
      <div class="space-y-2">
        <Label for="org-member-email">{{ $t('common.email') }} <span class="text-destructive">*</span></Label>
        <Input
          id="org-member-email"
          v-model="email"
          type="email"
          autocomplete="off"
          :placeholder="$t('orgMembers.emailPlaceholder')"
        />
        <p class="text-xs text-muted-foreground">{{ $t('orgMembers.emailHint') }}</p>
      </div>
      <div class="space-y-2">
        <Label>{{ $t('orgMembers.role') }} <span class="text-destructive">*</span></Label>
        <Select v-model="roleId">
          <SelectTrigger data-testid="add-member-role">
            <SelectValue :placeholder="$t('orgMembers.selectRole')" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem v-for="role in roles" :key="role.id" :value="role.id" :text-value="role.name">
              <span class="capitalize">{{ role.name }}</span>
            </SelectItem>
          </SelectContent>
        </Select>
      </div>
    </div>
  </CrudFormDialog>
</template>
