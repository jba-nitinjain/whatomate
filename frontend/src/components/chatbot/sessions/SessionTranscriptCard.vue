<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import type { ChatbotSessionMessage } from '@/services/api'
import { formatDateTime } from '@/lib/utils'
import { MessagesSquare, User, Bot } from 'lucide-vue-next'

defineProps<{ messages: ChatbotSessionMessage[] }>()
const { t } = useI18n()

// Internal step markers that mean nothing to a business user.
const HIDDEN_STEPS = new Set(['keyword_check', '__initial__'])

function stepLabel(step: string): string {
  if (!step || HIDDEN_STEPS.has(step)) return ''
  if (step === 'flow_complete') return t('chatbotSessions.status_completed')
  return step
}
</script>

<template>
  <Card>
    <CardHeader class="pb-3">
      <div class="flex items-center gap-2">
        <MessagesSquare class="h-4 w-4 text-muted-foreground" />
        <CardTitle class="text-sm font-medium">{{ t('chatbotSessions.transcript') }}</CardTitle>
      </div>
      <CardDescription>{{ t('chatbotSessions.transcriptDesc') }}</CardDescription>
    </CardHeader>
    <CardContent>
      <div v-if="messages.length" class="space-y-3" data-testid="session-transcript">
        <div
          v-for="msg in messages"
          :key="msg.id"
          class="flex"
          :class="msg.direction === 'outgoing' ? 'justify-end' : 'justify-start'"
        >
          <div
            class="max-w-[85%] rounded-lg px-3 py-2"
            :class="msg.direction === 'outgoing'
              ? 'bg-emerald-500/10 border border-emerald-500/20'
              : 'bg-muted/60 border border-white/[0.06] light:border-gray-200'"
          >
            <div class="flex items-center gap-1.5 text-xs text-muted-foreground mb-1">
              <Bot v-if="msg.direction === 'outgoing'" class="h-3 w-3" />
              <User v-else class="h-3 w-3" />
              <span class="font-medium">
                {{ msg.direction === 'outgoing' ? t('chatbotSessions.bot') : t('chatbotSessions.customer') }}
              </span>
              <span v-if="stepLabel(msg.step_name)">· {{ t('chatbotSessions.step') }}: {{ stepLabel(msg.step_name) }}</span>
            </div>
            <p class="text-sm whitespace-pre-wrap break-words">{{ msg.message }}</p>
            <p class="mt-1 text-[11px] text-muted-foreground text-right">{{ formatDateTime(msg.created_at) }}</p>
          </div>
        </div>
      </div>
      <p v-else class="text-sm text-muted-foreground">{{ t('chatbotSessions.noMessages') }}</p>
    </CardContent>
  </Card>
</template>
