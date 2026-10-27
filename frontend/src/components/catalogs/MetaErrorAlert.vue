<script setup lang="ts">
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { AlertTriangle } from 'lucide-vue-next'

defineProps<{ message: string | null }>()
</script>

<template>
  <Alert v-if="message" variant="destructive" data-testid="catalog-meta-error">
    <AlertTriangle class="h-4 w-4" />
    <AlertTitle>{{ $t('catalogs.metaErrorTitle') }}</AlertTitle>
    <AlertDescription class="break-words whitespace-pre-line">{{ message }}</AlertDescription>
  </Alert>
</template>
