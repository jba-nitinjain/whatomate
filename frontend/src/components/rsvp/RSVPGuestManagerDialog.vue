<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { rsvpService } from '@/services/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { CheckCircle, Download, Loader2, Upload, Users } from 'lucide-vue-next'

interface Candidate { id: string; profile_name: string; phone_number: string; tags?: string[]; already_added: boolean }
const props = defineProps<{ open: boolean; eventId: string }>()
const emit = defineEmits<{ 'update:open': [value: boolean]; changed: [] }>()
const { t } = useI18n()

const tab = ref('contacts')
const search = ref('')
const candidates = ref<Candidate[]>([])
const selected = ref<Set<string>>(new Set())
const loading = ref(false)
const importing = ref(false)
const file = ref<File | null>(null)
const importResult = ref<any>(null)
let searchTimer: number | undefined

async function loadCandidates() {
  loading.value = true
  try {
    const response = await rsvpService.guestCandidates(props.eventId, { search: search.value || undefined, limit: 50 })
    const data = (response.data as any).data || response.data
    candidates.value = data.contacts || []
  } finally { loading.value = false }
}

watch(() => props.open, value => {
  if (value) { selected.value = new Set(); importResult.value = null; loadCandidates() }
})
watch(search, () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = window.setTimeout(loadCandidates, 300)
})

function toggle(id: string) {
  const next = new Set(selected.value)
  if (next.has(id)) next.delete(id); else next.add(id)
  selected.value = next
}

async function addSelected() {
  if (!selected.value.size) return
  loading.value = true
  try {
    const response = await rsvpService.addGuests(props.eventId, [...selected.value])
    const data = (response.data as any).data || response.data
    toast.success(t('rsvp.guestsAdded', { count: data.added || 0 }))
    selected.value = new Set()
    await loadCandidates()
    emit('changed')
  } catch (error: any) { toast.error(error?.response?.data?.message || t('rsvp.guestAddFailed')) }
  finally { loading.value = false }
}

function onFile(event: Event) { file.value = (event.target as HTMLInputElement).files?.[0] || null; importResult.value = null }
async function importFile() {
  if (!file.value) return
  importing.value = true
  try {
    const response = await rsvpService.importGuests(props.eventId, file.value)
    importResult.value = (response.data as any).data || response.data
    toast.success(t('rsvp.guestsAdded', { count: importResult.value.guests_added || 0 }))
    emit('changed')
  } catch (error: any) { toast.error(error?.response?.data?.message || t('rsvp.guestImportFailed')) }
  finally { importing.value = false }
}
function sampleCSV() {
  const url = URL.createObjectURL(new Blob(['name,phone_number\nGuest Name,919999999999\n'], { type: 'text/csv' }))
  const link = document.createElement('a'); link.href = url; link.download = 'rsvp-guests-sample.csv'; link.click(); URL.revokeObjectURL(url)
}
</script>

<template>
  <Dialog :open="open" @update:open="emit('update:open', $event)">
    <DialogContent class="max-w-2xl">
      <DialogHeader><DialogTitle>{{ t('rsvp.manageGuests') }}</DialogTitle><DialogDescription>{{ t('rsvp.manageGuestsHint') }}</DialogDescription></DialogHeader>
      <Tabs v-model="tab">
        <TabsList class="grid w-full grid-cols-2"><TabsTrigger value="contacts">{{ t('rsvp.selectContacts') }}</TabsTrigger><TabsTrigger value="upload">{{ t('rsvp.uploadSpreadsheet') }}</TabsTrigger></TabsList>
        <TabsContent value="contacts" class="space-y-3">
          <Input v-model="search" :placeholder="t('rsvp.searchContacts')" />
          <div v-if="loading" class="flex justify-center p-6"><Loader2 class="h-5 w-5 animate-spin" /></div>
          <div v-else class="max-h-80 space-y-2 overflow-y-auto">
            <button v-for="contact in candidates" :key="contact.id" type="button" :disabled="contact.already_added" :class="['flex w-full items-start justify-between rounded-lg border p-3 text-left', selected.has(contact.id) ? 'border-primary bg-primary/5' : '', contact.already_added ? 'opacity-60' : 'hover:bg-muted/40']" @click="toggle(contact.id)">
              <div><div class="font-medium">{{ contact.profile_name || contact.phone_number }}</div><div class="text-xs text-muted-foreground">{{ contact.phone_number }}</div><div class="mt-1 flex gap-1"><Badge v-for="tag in contact.tags || []" :key="tag" variant="secondary">{{ tag }}</Badge></div></div>
              <span v-if="contact.already_added" class="text-xs text-muted-foreground">{{ t('rsvp.alreadyAdded') }}</span><CheckCircle v-else-if="selected.has(contact.id)" class="h-4 w-4 text-primary" /><Users v-else class="h-4 w-4 text-muted-foreground" />
            </button>
          </div>
          <Button class="w-full" :disabled="!selected.size || loading" @click="addSelected">{{ t('rsvp.addSelectedGuests', { count: selected.size }) }}</Button>
        </TabsContent>
        <TabsContent value="upload" class="space-y-4">
          <div class="rounded-lg border border-dashed p-5 text-sm"><input type="file" accept=".csv,.xlsx" @change="onFile" /><p class="mt-2 text-xs text-muted-foreground">{{ t('rsvp.importColumnsHint') }}</p></div>
          <div class="flex gap-2"><Button :disabled="!file || importing" @click="importFile"><Loader2 v-if="importing" class="mr-2 h-4 w-4 animate-spin" /><Upload v-else class="mr-2 h-4 w-4" />{{ t('rsvp.importGuests') }}</Button><Button variant="outline" @click="sampleCSV"><Download class="mr-2 h-4 w-4" />{{ t('rsvp.sampleCsv') }}</Button></div>
          <div v-if="importResult" class="rounded-lg border p-3 text-sm space-y-1"><p>{{ t('rsvp.importSummary', { added: importResult.guests_added, existing: importResult.already_added, created: importResult.contacts_created, skipped: importResult.skipped }) }}</p><ul v-if="importResult.errors?.length" class="max-h-32 overflow-y-auto text-xs text-destructive"><li v-for="e in importResult.errors" :key="`${e.row}-${e.message}`">{{ t('rsvp.rowError', { row: e.row, message: e.message }) }}</li></ul></div>
        </TabsContent>
      </Tabs>
      <DialogFooter><Button variant="outline" @click="emit('update:open', false)">{{ t('common.close') }}</Button></DialogFooter>
    </DialogContent>
  </Dialog>
</template>
