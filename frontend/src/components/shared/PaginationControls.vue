<script setup lang="ts">
import { computed } from 'vue'
import { Button } from '@/components/ui/button'
import {
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
} from 'lucide-vue-next'
import { getPageNumbers } from '@/composables/usePagination'

const props = defineProps<{
  currentPage: number
  totalPages: number
  totalItems: number
  pageSize: number
  itemName?: string
}>()

const emit = defineEmits<{
  'update:currentPage': [page: number]
}>()

const paginationInfo = computed(() => {
  const start = (props.currentPage - 1) * props.pageSize + 1
  const end = Math.min(props.currentPage * props.pageSize, props.totalItems)
  return { start, end }
})

const pageNumbers = computed(() => getPageNumbers(props.currentPage, props.totalPages))

function goToPage(page: number | '...') {
  if (page === '...') return
  emit('update:currentPage', page)
}
</script>

<template>
  <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
    <p class="text-sm text-muted-foreground">
      Showing {{ paginationInfo.start }} to {{ paginationInfo.end }} of {{ totalItems }} {{ itemName || 'items' }}
    </p>
    <div class="flex flex-wrap items-center gap-2 sm:justify-end">
      <Button
        variant="outline"
        size="icon"
        class="hidden h-8 w-8 sm:inline-flex"
        :disabled="currentPage === 1"
        @click="goToPage(1)"
      >
        <ChevronsLeft class="h-4 w-4" />
      </Button>
      <Button
        variant="outline"
        size="icon"
        class="h-8 w-8"
        :disabled="currentPage === 1"
        @click="goToPage(currentPage - 1)"
      >
        <ChevronLeft class="h-4 w-4" />
      </Button>
      <span class="px-2 text-xs text-muted-foreground sm:hidden">
        Page {{ currentPage }} of {{ totalPages }}
      </span>
      <div class="mx-2 hidden items-center gap-1 sm:flex">
        <template v-for="(page, index) in pageNumbers" :key="index">
          <Button
            v-if="page !== '...'"
            :variant="page === currentPage ? 'default' : 'outline'"
            size="icon"
            class="h-8 w-8"
            @click="goToPage(page)"
          >
            {{ page }}
          </Button>
          <span v-else class="px-1 text-muted-foreground">...</span>
        </template>
      </div>
      <Button
        variant="outline"
        size="icon"
        class="h-8 w-8"
        :disabled="currentPage === totalPages"
        @click="goToPage(currentPage + 1)"
      >
        <ChevronRight class="h-4 w-4" />
      </Button>
      <Button
        variant="outline"
        size="icon"
        class="h-8 w-8"
        :disabled="currentPage === totalPages"
        @click="goToPage(totalPages)"
      >
        <ChevronsRight class="h-4 w-4" />
      </Button>
    </div>
  </div>
</template>
