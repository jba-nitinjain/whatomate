<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { CrudFormDialog } from '@/components/shared'
import { organizationsService, type OrganizationMember, type Role } from '@/services/api'
import { getErrorMessage } from '@/lib/api-utils'
import { toast } from 'vue-sonner'

const props = defineProps<{ member: OrganizationMember | null; roles: Role[] }>()
const open = defineModel<boolean>('open', { default: false })
const emit = defineEmits<{ saved: [] }>()

const { t } = useI18n()
const roleId = ref('')
const isSubmitting = ref(false)

watch(open, (isOpen) => {
  if (isOpen) roleId.value = props.member?.role_id ?? ''
})

async function submit() {
  if (!props.member) return
  if (!roleId.value) {
    toast.error(t('orgMembers.roleRequired'))
    return
  }
  if (roleId.value === props.member.role_id) {
    open.value = false
    return
  }
  isSubmitting.value = true
  try {
    await organizationsService.updateMemberRole(props.member.user_id, roleId.value)
    toast.success(t('orgMembers.roleUpdated'))
    open.value = false
    emit('saved')
  } catch (e) {
    toast.error(getErrorMessage(e, t('orgMembers.roleUpdateFailed')))
  } finally {
    isSubmitting.value = false
  }
}
</script>

<template>
  <CrudFormDialog
    v-model:open="open"
    is-editing
    :is-submitting="isSubmitting"
    :edit-title="$t('orgMembers.changeRoleTitle')"
    :edit-description="$t('orgMembers.changeRoleDesc', { name: member?.full_name || member?.email || '' })"
    :edit-submit-label="$t('common.save')"
    :cancel-label="$t('common.cancel')"
    max-width="max-w-md"
    @submit="submit"
  >
    <div class="space-y-2">
      <Label>{{ $t('orgMembers.role') }} <span class="text-destructive">*</span></Label>
      <Select v-model="roleId">
        <SelectTrigger data-testid="change-member-role">
          <SelectValue :placeholder="$t('orgMembers.selectRole')" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem v-for="role in roles" :key="role.id" :value="role.id" :text-value="role.name">
            <span class="capitalize">{{ role.name }}</span>
          </SelectItem>
        </SelectContent>
      </Select>
      <p class="text-xs text-muted-foreground">{{ $t('orgMembers.changeRoleHint') }}</p>
    </div>
  </CrudFormDialog>
</template>
