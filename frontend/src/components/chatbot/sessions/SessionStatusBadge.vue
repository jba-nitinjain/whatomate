<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Badge } from '@/components/ui/badge'

const props = defineProps<{ status: string }>()
const { t, te } = useI18n()

const STATUS_CLASSES: Record<string, string> = {
  active: 'bg-blue-500/10 text-blue-500 border-blue-500/20',
  completed: 'bg-green-500/10 text-green-500 border-green-500/20',
  cancelled: 'bg-gray-500/10 text-gray-400 border-gray-500/20',
  timeout: 'bg-amber-500/10 text-amber-500 border-amber-500/20',
}

const label = computed(() => {
  const key = `chatbotSessions.status_${props.status}`
  return te(key) ? t(key) : props.status
})
</script>

<template>
  <Badge variant="outline" :class="[STATUS_CLASSES[status] || '', 'text-xs']">
    {{ label }}
  </Badge>
</template>
