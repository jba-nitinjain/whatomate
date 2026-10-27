<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import SessionStatusBadge from './SessionStatusBadge.vue'
import type { ChatbotSession } from '@/services/api'
import { formatDateTime } from '@/lib/utils'
import { Info, MessageSquare } from 'lucide-vue-next'

defineProps<{ session: ChatbotSession; contactName: string }>()
const { t } = useI18n()
</script>

<template>
  <Card class="overflow-hidden">
    <CardHeader class="pb-3">
      <div class="flex items-center gap-2">
        <Info class="h-4 w-4 text-muted-foreground" />
        <CardTitle class="text-sm font-medium">{{ t('chatbotSessions.details') }}</CardTitle>
      </div>
    </CardHeader>
    <CardContent class="space-y-3 text-sm" data-testid="session-details">
      <div>
        <span class="text-muted-foreground text-xs">{{ t('chatbotSessions.contact') }}</span>
        <p class="font-medium">{{ contactName }}</p>
        <p class="text-xs text-muted-foreground">{{ session.phone_number }}</p>
      </div>
      <div>
        <span class="text-muted-foreground text-xs">{{ t('chatbotSessions.status') }}</span>
        <div class="mt-0.5"><SessionStatusBadge :status="session.status" /></div>
      </div>
      <div>
        <span class="text-muted-foreground text-xs">{{ t('chatbotSessions.flow') }}</span>
        <p class="font-medium">{{ session.flow_name || t('chatbotSessions.noFlow') }}</p>
      </div>
      <div v-if="session.current_step">
        <span class="text-muted-foreground text-xs">{{ t('chatbotSessions.currentStep') }}</span>
        <p>{{ session.current_step }}</p>
      </div>
      <div v-if="session.step_retries > 0">
        <span class="text-muted-foreground text-xs">{{ t('chatbotSessions.retries') }}</span>
        <p>{{ session.step_retries }}</p>
      </div>
      <div v-if="session.whatsapp_account">
        <span class="text-muted-foreground text-xs">{{ t('chatbotSessions.account') }}</span>
        <p>{{ session.whatsapp_account }}</p>
      </div>
      <div>
        <span class="text-muted-foreground text-xs">{{ t('chatbotSessions.startedAt') }}</span>
        <p>{{ formatDateTime(session.started_at) }}</p>
      </div>
      <div>
        <span class="text-muted-foreground text-xs">{{ t('chatbotSessions.lastActivity') }}</span>
        <p>{{ formatDateTime(session.last_activity_at) }}</p>
      </div>
      <div>
        <span class="text-muted-foreground text-xs">{{ t('chatbotSessions.endedAt') }}</span>
        <p v-if="session.completed_at">{{ formatDateTime(session.completed_at) }}</p>
        <p v-else-if="session.status === 'active'" class="text-muted-foreground">{{ t('chatbotSessions.notEnded') }}</p>
        <p v-else class="text-muted-foreground">—</p>
      </div>
      <RouterLink :to="`/chat/${session.contact_id}`" data-testid="session-open-chat">
        <Button variant="outline" size="sm" class="w-full mt-2">
          <MessageSquare class="h-3.5 w-3.5 mr-1.5" />
          {{ t('chatbotSessions.openChat') }}
        </Button>
      </RouterLink>
    </CardContent>
  </Card>
</template>
