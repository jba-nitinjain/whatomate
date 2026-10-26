<script setup lang="ts">
import { ref, onMounted, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { TagBadge } from '@/components/ui/tag-badge'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { PageHeader, SearchInput, DataTable, DeleteConfirmDialog, CreateContactDialog, ImportExportDialog, IconButton, ErrorState, RefreshButton, type Column } from '@/components/shared'
import { contactsService, accountsService, type Tag, type ImportResult } from '@/services/api'
import { useTagsStore } from '@/stores/tags'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { toast } from 'vue-sonner'
import { Plus, Users, Pencil, Trash2, MessageSquare, Download, Check, ChevronsUpDown, X } from 'lucide-vue-next'
import { getTagColorClass } from '@/lib/constants'
import { getErrorMessage } from '@/lib/api-utils'
import { formatDate } from '@/lib/utils'
import { useSearchPagination } from '@/composables/useSearchPagination'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useViewRefresh } from '@/composables/useViewRefresh'

const { t } = useI18n()
const router = useRouter()
const authStore = useAuthStore()
const tagsStore = useTagsStore()
const canWriteContacts = authStore.hasPermission('contacts', 'write')
const canImportContacts = authStore.hasPermission('contacts', 'import')
const canExportContacts = authStore.hasPermission('contacts', 'export')

// Import/Export dialog state
const isImportExportOpen = ref(false)

interface Contact {
  id: string
  phone_number: string
  profile_name: string
  name: string
  whatsapp_account: string
  tags: string[]
  metadata: Record<string, any>
  assigned_user_id: string | null
  last_message_at: string | null
  last_message_preview: string
  unread_count: number
  is_active: boolean
  created_at: string
  updated_at: string
}

const contacts = ref<Contact[]>([])
const availableAccounts = ref<{ id: string; name: string; phone_number: string }[]>([])
const isLoading = ref(false)
const isDeleting = ref(false)
const error = ref(false)
const isCreateDialogOpen = ref(false)
const deleteDialogOpen = ref(false)
const contactToDelete = ref<Contact | null>(null)
const activeFilter = ref<'all' | 'active' | 'inactive'>('all')
const accountFilter = ref('all')
const selectedFilterTags = ref<string[]>([])
const filterTagSelectorOpen = ref(false)
const availableTags = ref<Tag[]>([])

// Sorting state
const sortKey = ref('last_message_at')
const sortDirection = ref<'asc' | 'desc'>('desc')

const filterTagsParam = computed(() => selectedFilterTags.value.length > 0 ? selectedFilterTags.value.join(',') : undefined)
const exportFilters = computed<Record<string, string>>(() => ({
  ...(searchQuery.value ? { search: searchQuery.value } : {}),
  ...(activeFilter.value !== 'all' ? { is_active: String(activeFilter.value === 'active') } : {}),
  ...(accountFilter.value !== 'all' ? { whatsapp_account: accountFilter.value } : {}),
  ...(filterTagsParam.value ? { tags: filterTagsParam.value } : {})
}))

const columns = computed<Column<Contact>[]>(() => [
  { key: 'profile_name', label: t('contacts.name'), sortable: true },
  { key: 'phone_number', label: t('contacts.phoneNumber'), sortable: true },
  { key: 'status', label: t('common.status') },
  { key: 'tags', label: t('contacts.tags') },
  { key: 'last_message_at', label: t('contacts.lastMessage'), sortable: true },
  { key: 'created_at', label: t('contacts.created'), sortable: true },
  { key: 'actions', label: t('common.actions'), align: 'right' },
])

function openCreateDialog() {
  isCreateDialogOpen.value = true
}

function onContactCreated() {
  fetchContacts()
}

function onImported(_result: ImportResult) {
  // Refresh the contacts list but keep dialog open to show import results
  fetchContacts()
  // Dialog stays open so user can see import results
}

function openDeleteDialog(contact: Contact) {
  contactToDelete.value = contact
  deleteDialogOpen.value = true
}

function closeDeleteDialog() {
  deleteDialogOpen.value = false
  contactToDelete.value = null
}

async function fetchContacts() {
  isLoading.value = true
  error.value = false
  try {
    const response = await contactsService.list({
      search: searchQuery.value || undefined,
      is_active: activeFilter.value === 'all' ? undefined : activeFilter.value === 'active',
      whatsapp_account: accountFilter.value === 'all' ? undefined : accountFilter.value,
      tags: filterTagsParam.value,
      page: currentPage.value,
      limit: pageSize
    })
    const data = response.data as any
    const responseData = data.data || data
    contacts.value = responseData.contacts || []
    totalItems.value = responseData.total ?? contacts.value.length
  } catch (e) {
    toast.error(getErrorMessage(e, t('common.failedLoad', { resource: t('resources.contacts') })))
    error.value = true
  } finally {
    isLoading.value = false
  }
}

const { isRefreshing: isRefreshingView, refreshNow } = useViewRefresh(fetchContacts)

async function fetchTags() {
  try {
    const response = await tagsStore.fetchTags({ limit: 100 })
    availableTags.value = response.tags
  } catch {
    // Silently fail - tags are optional
  }
}

async function fetchAccounts() {
  try {
    const response = await accountsService.list()
    const data = response.data as any
    const responseData = data.data || data
    availableAccounts.value = responseData.accounts || []
  } catch (error) {
    // Silently fail - accounts are optional
  }
}

const { searchQuery, currentPage, totalItems, pageSize, handlePageChange } = useSearchPagination({
  fetchFn: () => fetchContacts(),
})

watch(activeFilter, () => {
  currentPage.value = 1
  fetchContacts()
})
watch(accountFilter, () => {
  currentPage.value = 1
  fetchContacts()
})
watch(selectedFilterTags, () => {
  currentPage.value = 1
  fetchContacts()
}, { deep: true })

onMounted(() => {
  fetchContacts()
  fetchAccounts()
  fetchTags()
})

async function confirmDelete() {
  if (!contactToDelete.value) return
  isDeleting.value = true
  try {
    await contactsService.delete(contactToDelete.value.id)
    toast.success(t('common.deletedSuccess', { resource: t('resources.Contact') }))
    closeDeleteDialog()
    await fetchContacts()
  } catch (e) {
    toast.error(getErrorMessage(e, t('common.failedDelete', { resource: t('resources.contact') })))
  } finally {
    isDeleting.value = false
  }
}

function openChat(contact: Contact) {
  router.push({ name: 'chat-conversation', params: { contactId: contact.id } })
}

function toggleFilterTag(tagName: string) {
  const index = selectedFilterTags.value.indexOf(tagName)
  if (index === -1) {
    selectedFilterTags.value.push(tagName)
  } else {
    selectedFilterTags.value.splice(index, 1)
  }
}

function removeFilterTag(tagName: string) {
  selectedFilterTags.value = selectedFilterTags.value.filter(t => t !== tagName)
}

function isFilterTagSelected(tagName: string): boolean {
  return selectedFilterTags.value.includes(tagName)
}

function getTagDetails(tagName: string): Tag | undefined {
  return availableTags.value.find(t => t.name === tagName)
}

function getDisplayName(contact: Contact): string {
  return contact.profile_name || contact.name || contact.phone_number
}
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="$t('contacts.title')" :subtitle="$t('contacts.subtitle')" :icon="Users" icon-gradient="bg-gradient-to-br from-blue-500 to-cyan-600 shadow-blue-500/20" back-link="/settings">
      <template v-if="canWriteContacts || canImportContacts || canExportContacts" #actions>
        <RefreshButton :refreshing="isRefreshingView" :label="$t('common.refresh')" @refresh="refreshNow(true)" />
        <Button v-if="canImportContacts || canExportContacts" variant="outline" size="sm" @click="isImportExportOpen = true">
          <Download class="h-4 w-4 mr-2" />{{ $t('common.import') }}/{{ $t('common.export') }}
        </Button>
        <Button v-if="canWriteContacts" variant="outline" size="sm" @click="openCreateDialog"><Plus class="h-4 w-4 mr-2" />{{ $t('contacts.addContact') }}</Button>
      </template>
      <template v-else #actions>
        <RefreshButton :refreshing="isRefreshingView" :label="$t('common.refresh')" @refresh="refreshNow(true)" />
      </template>
    </PageHeader>

    <!-- Error State -->
    <ErrorState
      v-if="error && !isLoading"
      :title="$t('common.loadErrorTitle')"
      :description="$t('common.loadErrorDescription')"
      :retry-label="$t('common.retryLoad')"
      class="flex-1"
      @retry="fetchContacts"
    />

    <ScrollArea v-else class="flex-1">
      <div class="p-6">
        <div>
          <Card>
            <CardHeader>
              <div class="flex items-center justify-between flex-wrap gap-4">
                <div>
                  <CardTitle>{{ $t('contacts.allContacts') }}</CardTitle>
                  <CardDescription>{{ $t('contacts.allContactsDesc') }}</CardDescription>
                </div>
                <div class="flex flex-wrap items-center gap-2">
                  <Select v-model="activeFilter">
                    <SelectTrigger class="w-[150px]">
                      <SelectValue :placeholder="$t('common.status')" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">{{ $t('common.all') }}</SelectItem>
                      <SelectItem value="active">{{ $t('common.active') }}</SelectItem>
                      <SelectItem value="inactive">{{ $t('common.inactive') }}</SelectItem>
                    </SelectContent>
                  </Select>
                  <Select v-if="availableAccounts.length > 0" v-model="accountFilter">
                    <SelectTrigger class="w-[190px]">
                      <SelectValue :placeholder="$t('contacts.whatsappAccount')" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">{{ $t('common.all') }}</SelectItem>
                      <SelectItem v-for="account in availableAccounts" :key="account.id" :value="account.name">
                        {{ account.name }}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                  <Popover v-if="availableTags.length > 0" v-model:open="filterTagSelectorOpen">
                    <PopoverTrigger as-child>
                      <Button variant="outline" class="w-[160px] justify-between">
                        <span v-if="selectedFilterTags.length === 0">{{ $t('contacts.tags') }}</span>
                        <span v-else>{{ selectedFilterTags.length }} {{ $t('contacts.tagsSelected') }}</span>
                        <ChevronsUpDown class="ml-2 h-4 w-4 shrink-0 opacity-50" />
                      </Button>
                    </PopoverTrigger>
                    <PopoverContent class="w-[300px] p-0" @interact-outside="(e) => e.preventDefault()">
                      <Command>
                        <CommandInput :placeholder="$t('contacts.searchTags')" />
                        <CommandList>
                          <CommandEmpty>{{ $t('contacts.noTagsFound') }}</CommandEmpty>
                          <CommandGroup>
                            <CommandItem
                              v-for="tag in availableTags"
                              :key="tag.name"
                              :value="tag.name"
                              class="flex items-center gap-2 cursor-pointer"
                              @select.prevent="toggleFilterTag(tag.name)"
                            >
                              <div class="flex items-center gap-2 flex-1">
                                <span :class="['w-2 h-2 rounded-full', getTagColorClass(tag.color).split(' ')[0]]"></span>
                                <span>{{ tag.name }}</span>
                              </div>
                              <Check v-if="isFilterTagSelected(tag.name)" class="h-4 w-4 text-primary" />
                            </CommandItem>
                          </CommandGroup>
                        </CommandList>
                      </Command>
                    </PopoverContent>
                  </Popover>
                  <SearchInput v-model="searchQuery" :placeholder="$t('contacts.searchContacts') + '...'" class="w-64" />
                </div>
              </div>
              <div v-if="selectedFilterTags.length > 0" class="flex flex-wrap gap-1 pt-3">
                <TagBadge
                  v-for="tagName in selectedFilterTags"
                  :key="tagName"
                  :color="getTagDetails(tagName)?.color"
                >
                  {{ tagName }}
                  <button
                    type="button"
                    class="ml-1 rounded-full hover:bg-black/10 dark:hover:bg-white/10 p-0.5 transition-colors"
                    @click.stop="removeFilterTag(tagName)"
                  >
                    <X class="h-3 w-3" />
                  </button>
                </TagBadge>
              </div>
            </CardHeader>
            <CardContent>
              <DataTable
                :items="contacts"
                :columns="columns"
                :is-loading="isLoading"
                :empty-icon="Users"
                :empty-title="searchQuery ? $t('contacts.noMatchingContacts') : $t('contacts.noContactsYet')"
                :empty-description="searchQuery ? $t('contacts.noMatchingContactsDesc') : $t('contacts.noContactsYetDesc')"
                v-model:sort-key="sortKey"
                v-model:sort-direction="sortDirection"
                server-pagination
                :current-page="currentPage"
                :total-items="totalItems"
                :page-size="pageSize"
                item-name="contacts"
                @page-change="handlePageChange"
              >
                <template #cell-profile_name="{ item: contact }">
                  <div class="flex flex-col">
                    <RouterLink :to="`/settings/contacts/${contact.id}`" class="font-medium text-inherit no-underline hover:opacity-80">{{ getDisplayName(contact) }}</RouterLink>
                    <span v-if="contact.last_message_preview" class="text-xs text-muted-foreground truncate max-w-[200px]">{{ contact.last_message_preview }}</span>
                  </div>
                </template>
                <template #cell-phone_number="{ item: contact }">
                  <code class="text-sm">{{ contact.phone_number }}</code>
                </template>
                <template #cell-status="{ item: contact }">
                  <Badge :variant="contact.is_active ? 'default' : 'secondary'" class="text-xs">
                    {{ contact.is_active ? $t('common.active') : $t('common.inactive') }}
                  </Badge>
                </template>
                <template #cell-tags="{ item: contact }">
                  <div class="flex flex-wrap gap-1">
                    <TagBadge v-for="tag in (contact.tags || []).slice(0, 3)" :key="tag" color="gray" class="text-xs">{{ tag }}</TagBadge>
                    <Badge v-if="(contact.tags || []).length > 3" variant="outline" class="text-xs">+{{ contact.tags.length - 3 }}</Badge>
                  </div>
                </template>
                <template #cell-last_message_at="{ item: contact }">
                  <span class="text-muted-foreground">{{ contact.last_message_at ? formatDate(contact.last_message_at) : $t('contacts.never') }}</span>
                </template>
                <template #cell-created_at="{ item: contact }">
                  <span class="text-muted-foreground">{{ formatDate(contact.created_at) }}</span>
                </template>
                <template #cell-actions="{ item: contact }">
                  <div class="flex items-center justify-end gap-1">
                    <IconButton :icon="MessageSquare" :label="$t('contacts.openChat')" class="h-8 w-8" @click="openChat(contact)" />
                    <RouterLink :to="`/settings/contacts/${contact.id}`">
                      <IconButton :icon="Pencil" :label="$t('common.edit')" class="h-8 w-8" />
                    </RouterLink>
                    <IconButton :label="$t('common.delete')" class="h-8 w-8" @click="openDeleteDialog(contact)">
                      <Trash2 class="h-4 w-4 text-destructive" />
                    </IconButton>
                  </div>
                </template>
                <template v-if="canWriteContacts" #empty-action>
                  <Button variant="outline" size="sm" @click="openCreateDialog">
                    <Plus class="h-4 w-4 mr-2" />
                    {{ $t('contacts.addContact') }}
                  </Button>
                </template>
              </DataTable>
            </CardContent>
          </Card>
        </div>
      </div>
    </ScrollArea>

    <!-- Create Contact Dialog (shared component) -->
    <CreateContactDialog v-model:open="isCreateDialogOpen" @created="onContactCreated" />

    <DeleteConfirmDialog
      v-model:open="deleteDialogOpen"
      :title="$t('contacts.deleteContact')"
      :item-name="contactToDelete ? getDisplayName(contactToDelete) : ''"
      :description="$t('contacts.deleteWarning')"
      :is-submitting="isDeleting"
      @confirm="confirmDelete"
    />

    <ImportExportDialog
      v-model:open="isImportExportOpen"
      table="contacts"
      :table-label="$t('contacts.title')"
      :filters="exportFilters"
      :can-import="canImportContacts"
      :can-export="canExportContacts"
      @imported="onImported"
    />
  </div>
</template>
