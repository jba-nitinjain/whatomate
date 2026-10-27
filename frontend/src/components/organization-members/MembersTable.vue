<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable, IconButton, type Column } from '@/components/shared'
import { Building, Home, ShieldCheck, Shield, UserCog, UserMinus, UserPlus, Users } from 'lucide-vue-next'
import type { OrganizationMember } from '@/services/api'
import { ROLE_BADGE_VARIANTS } from '@/lib/constants'
import { formatDate } from '@/lib/utils'

const props = defineProps<{
  members: OrganizationMember[]
  isLoading: boolean
  searchQuery: string
  currentPage: number
  totalItems: number
  pageSize: number
  currentUserId?: string
  canManage: boolean
}>()

const emit = defineEmits<{
  'page-change': [page: number]
  'change-role': [member: OrganizationMember]
  remove: [member: OrganizationMember]
  add: []
}>()

const { t } = useI18n()
const sortKey = ref('full_name')
const sortDirection = ref<'asc' | 'desc'>('asc')

const columns = computed<Column<OrganizationMember>[]>(() => {
  const cols: Column<OrganizationMember>[] = [
    { key: 'member', label: t('orgMembers.member'), width: 'w-[280px]', sortable: true, sortKey: 'full_name' },
    { key: 'email', label: t('common.email'), sortable: true, sortKey: 'email' },
    { key: 'role', label: t('orgMembers.role'), sortable: true, sortKey: 'role_name' },
    { key: 'home', label: t('orgMembers.homeOrganization'), sortable: true, sortKey: 'is_default' },
    { key: 'joined', label: t('orgMembers.joined'), sortable: true, sortKey: 'created_at' },
  ]
  if (props.canManage) cols.push({ key: 'actions', label: t('common.actions'), align: 'right' })
  return cols
})

const isSelf = (m: OrganizationMember) => m.user_id === props.currentUserId
const roleName = (m: OrganizationMember) => m.role_name || t('orgMembers.noRole')
function roleVariant(m: OrganizationMember): 'default' | 'secondary' | 'outline' {
  return ROLE_BADGE_VARIANTS[(m.role_name || '').toLowerCase()] || 'outline'
}
function roleIcon(m: OrganizationMember) {
  return { admin: ShieldCheck, manager: Shield }[(m.role_name || '').toLowerCase()] || UserCog
}
</script>

<template>
  <DataTable
    :items="members"
    :columns="columns"
    :is-loading="isLoading"
    :empty-icon="Users"
    :empty-title="searchQuery ? $t('orgMembers.noMatchingMembers') : $t('orgMembers.noMembers')"
    :empty-description="searchQuery ? $t('orgMembers.noMatchingMembersDesc') : $t('orgMembers.noMembersDesc')"
    v-model:sort-key="sortKey"
    v-model:sort-direction="sortDirection"
    server-pagination
    :current-page="currentPage"
    :total-items="totalItems"
    :page-size="pageSize"
    item-name="members"
    @page-change="emit('page-change', $event)"
  >
    <template #cell-member="{ item: m }">
      <div class="flex items-center gap-3">
        <div class="h-9 w-9 rounded-full bg-primary/10 flex items-center justify-center flex-shrink-0">
          <component :is="roleIcon(m)" class="h-4 w-4 text-primary" />
        </div>
        <div class="min-w-0">
          <div class="flex items-center gap-2 flex-wrap">
            <p class="font-medium truncate">{{ m.full_name || m.email }}</p>
            <Badge v-if="isSelf(m)" variant="outline" class="text-xs">{{ $t('orgMembers.you') }}</Badge>
            <Badge v-if="!m.is_active" variant="outline" class="text-xs text-muted-foreground">{{ $t('common.inactive') }}</Badge>
          </div>
        </div>
      </div>
    </template>
    <template #cell-email="{ item: m }">
      <span class="text-sm text-muted-foreground truncate">{{ m.email }}</span>
    </template>
    <template #cell-role="{ item: m }">
      <Badge :variant="roleVariant(m)" class="capitalize">{{ roleName(m) }}</Badge>
    </template>
    <template #cell-home="{ item: m }">
      <span v-if="m.is_default" class="inline-flex items-center gap-1.5 text-sm text-emerald-600">
        <Home class="h-3.5 w-3.5" />{{ $t('orgMembers.homeYes') }}
      </span>
      <span v-else class="inline-flex items-center gap-1.5 text-sm text-muted-foreground">
        <Building class="h-3.5 w-3.5" />{{ $t('orgMembers.homeNo') }}
      </span>
    </template>
    <template #cell-joined="{ item: m }">
      <span class="text-muted-foreground">{{ formatDate(m.created_at) }}</span>
    </template>
    <template #cell-actions="{ item: m }">
      <div class="flex items-center justify-end gap-1">
        <IconButton
          :icon="UserCog"
          :label="isSelf(m) ? $t('orgMembers.cantChangeOwnRole') : $t('orgMembers.changeRole')"
          class="h-8 w-8"
          :disabled="isSelf(m)"
          @click="emit('change-role', m)"
        />
        <IconButton
          :label="isSelf(m) ? $t('orgMembers.cantRemoveYourself') : $t('orgMembers.removeMember')"
          class="h-8 w-8"
          :disabled="isSelf(m)"
          @click="emit('remove', m)"
        >
          <UserMinus class="h-4 w-4 text-destructive" />
        </IconButton>
      </div>
    </template>
    <template v-if="canManage && !searchQuery" #empty-action>
      <Button variant="outline" size="sm" @click="emit('add')">
        <UserPlus class="h-4 w-4 mr-2" />{{ $t('orgMembers.addMember') }}
      </Button>
    </template>
  </DataTable>
</template>
