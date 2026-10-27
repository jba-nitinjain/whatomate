<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Button } from '@/components/ui/button'
import { PageHeader, SearchInput, DeleteConfirmDialog, ErrorState, RefreshButton } from '@/components/shared'
import CurrentOrganizationCard from '@/components/organization-members/CurrentOrganizationCard.vue'
import MembersTable from '@/components/organization-members/MembersTable.vue'
import AddMemberDialog from '@/components/organization-members/AddMemberDialog.vue'
import ChangeRoleDialog from '@/components/organization-members/ChangeRoleDialog.vue'
import { organizationsService, type Organization, type OrganizationMember, type OrganizationMembersResponse } from '@/services/api'
import { useAuthStore } from '@/stores/auth'
import { useRolesStore } from '@/stores/roles'
import { useOrganizationsStore } from '@/stores/organizations'
import { toast } from 'vue-sonner'
import { Building2, UserPlus } from 'lucide-vue-next'
import { getErrorMessage } from '@/lib/api-utils'
import { useSearchPagination } from '@/composables/useSearchPagination'
import { useViewRefresh } from '@/composables/useViewRefresh'

const { t } = useI18n()
const authStore = useAuthStore()
const rolesStore = useRolesStore()
const organizationsStore = useOrganizationsStore()

const members = ref<OrganizationMember[]>([])
const organization = ref<Organization | null>(null)
const isLoading = ref(false)
const isOrgLoading = ref(false)
// Unfiltered member total for the summary card; the table total follows the search.
const memberCount = ref(0)
const error = ref(false)

const isAddOpen = ref(false)
const isRoleOpen = ref(false)
const memberForRole = ref<OrganizationMember | null>(null)
const removeOpen = ref(false)
const memberToRemove = ref<OrganizationMember | null>(null)
const isRemoving = ref(false)

const canManage = computed(() => authStore.hasPermission('organizations', 'assign'))
const currentUserId = computed(() => authStore.user?.id)
const breadcrumbs = computed(() => [{ label: t('nav.settings'), href: '/settings' }, { label: t('nav.orgMembers') }])

const { searchQuery, currentPage, totalItems, pageSize, handlePageChange } = useSearchPagination({
  fetchFn: () => fetchMembers(),
})

function unwrap<T>(data: unknown): T {
  return ((data as { data?: T })?.data ?? data) as T
}

async function fetchMembers() {
  isLoading.value = true
  error.value = false
  try {
    const response = await organizationsService.listMembers({
      search: searchQuery.value || undefined,
      page: currentPage.value,
      limit: pageSize,
    })
    const data = unwrap<OrganizationMembersResponse>(response.data)
    members.value = data.members || []
    totalItems.value = data.total ?? members.value.length
    if (!searchQuery.value) memberCount.value = totalItems.value
  } catch (e) {
    toast.error(getErrorMessage(e, t('common.failedLoad', { resource: t('orgMembers.resource') })))
    error.value = true
  } finally {
    isLoading.value = false
  }
}

async function fetchOrganization() {
  isOrgLoading.value = true
  try {
    const response = await organizationsService.getCurrent()
    organization.value = unwrap<Organization>(response.data)
  } catch {
    // The summary card is informational; the member list carries its own error state.
    organization.value = null
  } finally {
    isOrgLoading.value = false
  }
}

async function loadAll() {
  await Promise.all([fetchMembers(), fetchOrganization(), canManage.value ? rolesStore.fetchRoles({ limit: 100 }).catch(() => undefined) : undefined])
}

const { isRefreshing: isRefreshingView, refreshNow } = useViewRefresh(loadAll)

onMounted(loadAll)
watch(() => organizationsStore.selectedOrgId, () => { currentPage.value = 1; loadAll() })

function openChangeRole(member: OrganizationMember) {
  memberForRole.value = member
  isRoleOpen.value = true
}

function openRemove(member: OrganizationMember) {
  memberToRemove.value = member
  removeOpen.value = true
}

async function confirmRemove() {
  if (!memberToRemove.value) return
  isRemoving.value = true
  try {
    await organizationsService.removeMember(memberToRemove.value.user_id)
    toast.success(t('orgMembers.memberRemoved'))
    removeOpen.value = false
    memberToRemove.value = null
    await fetchMembers()
  } catch (e) {
    toast.error(getErrorMessage(e, t('orgMembers.removeFailed')))
  } finally {
    isRemoving.value = false
  }
}
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="$t('orgMembers.title')" :subtitle="$t('orgMembers.subtitle')" :icon="Building2" icon-gradient="bg-gradient-to-br from-teal-500 to-emerald-600 shadow-teal-500/20" back-link="/settings" :breadcrumbs="breadcrumbs">
      <template #actions>
        <RefreshButton :refreshing="isRefreshingView" :label="$t('common.refresh')" @refresh="refreshNow(true)" />
        <Button v-if="canManage" variant="outline" size="sm" @click="isAddOpen = true"><UserPlus class="h-4 w-4 mr-2" />{{ $t('orgMembers.addMember') }}</Button>
      </template>
    </PageHeader>

    <ErrorState
      v-if="error && !isLoading"
      :title="$t('common.loadErrorTitle')"
      :description="$t('common.loadErrorDescription')"
      :retry-label="$t('common.retryLoad')"
      class="flex-1"
      @retry="loadAll"
    />

    <ScrollArea v-else class="flex-1">
      <div class="p-6">
        <div class="max-w-6xl mx-auto space-y-4">
          <CurrentOrganizationCard :organization="organization" :member-count="memberCount" :loading="isOrgLoading" />
          <Card>
            <CardHeader>
              <div class="flex items-center justify-between flex-wrap gap-4">
                <div>
                  <CardTitle>{{ $t('orgMembers.listTitle') }}</CardTitle>
                  <CardDescription>{{ canManage ? $t('orgMembers.listDesc') : $t('orgMembers.listDescReadOnly') }}</CardDescription>
                </div>
                <SearchInput v-model="searchQuery" :placeholder="$t('orgMembers.searchPlaceholder')" class="w-64" />
              </div>
            </CardHeader>
            <CardContent>
              <MembersTable
                :members="members"
                :is-loading="isLoading"
                :search-query="searchQuery"
                :current-page="currentPage"
                :total-items="totalItems"
                :page-size="pageSize"
                :current-user-id="currentUserId"
                :can-manage="canManage"
                @page-change="handlePageChange"
                @change-role="openChangeRole"
                @remove="openRemove"
                @add="isAddOpen = true"
              />
            </CardContent>
          </Card>
        </div>
      </div>
    </ScrollArea>

    <AddMemberDialog v-model:open="isAddOpen" :roles="rolesStore.roles" @added="fetchMembers" />
    <ChangeRoleDialog v-model:open="isRoleOpen" :member="memberForRole" :roles="rolesStore.roles" @saved="fetchMembers" />

    <DeleteConfirmDialog
      v-model:open="removeOpen"
      :title="$t('orgMembers.removeTitle')"
      :confirm-label="$t('orgMembers.removeConfirm')"
      :cancel-label="$t('common.cancel')"
      :is-submitting="isRemoving"
      @confirm="confirmRemove"
    >
      <template #description>
        {{ $t('orgMembers.removeDesc', { name: memberToRemove?.full_name || memberToRemove?.email || '' }) }}
      </template>
    </DeleteConfirmDialog>
  </div>
</template>
