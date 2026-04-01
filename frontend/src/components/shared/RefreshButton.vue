<script setup lang="ts">
import { Button } from '@/components/ui/button'
import { Loader2, RefreshCw } from 'lucide-vue-next'

withDefaults(defineProps<{
  refreshing?: boolean
  label?: string
}>(), {
  refreshing: false,
  label: 'Refresh'
})

const emit = defineEmits<{
  refresh: []
}>()
</script>

<template>
  <Button variant="outline" size="sm" :disabled="refreshing" @click="emit('refresh')">
    <Loader2 v-if="refreshing" class="mr-2 h-4 w-4 animate-spin" />
    <RefreshCw v-else class="mr-2 h-4 w-4" />
    {{ label }}
  </Button>
</template>
