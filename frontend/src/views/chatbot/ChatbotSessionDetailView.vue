<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import DetailPageLayout from '@/components/shared/DetailPageLayout.vue'
import { RefreshButton } from '@/components/shared'
import SessionAnswersCard from '@/components/chatbot/sessions/SessionAnswersCard.vue'
import SessionTranscriptCard from '@/components/chatbot/sessions/SessionTranscriptCard.vue'
import SessionInfoCard from '@/components/chatbot/sessions/SessionInfoCard.vue'
import { chatbotSessionsService, type ChatbotSession } from '@/services/api'
import { getErrorMessage } from '@/lib/api-utils'
import { toast } from 'vue-sonner'
import { History } from 'lucide-vue-next'

const route = useRoute()
const { t } = useI18n()

const sessionId = computed(() => route.params.id as string)
const session = ref<ChatbotSession | null>(null)
const isLoading = ref(true)
const isRefreshing = ref(false)

const contactName = computed(() =>
  session.value?.contact?.profile_name ||
  session.value?.contact?.phone_number ||
  session.value?.phone_number ||
  t('chatbotSessions.unknownContact')
)

const title = computed(() => (session.value ? contactName.value : t('chatbotSessions.title')))

const breadcrumbs = computed(() => [
  { label: t('nav.chatbot'), href: '/chatbot' },
  { label: t('chatbotSessions.title'), href: '/chatbot/sessions' },
  { label: title.value },
])

async function fetchSession() {
  try {
    const response = await chatbotSessionsService.get(sessionId.value)
    session.value = (response.data as any).data || response.data
  } catch (err: any) {
    if (err?.response?.status !== 404) {
      toast.error(getErrorMessage(err, t('chatbotSessions.fetchError')))
    }
  }
}

async function refresh() {
  isRefreshing.value = true
  try {
    await fetchSession()
  } finally {
    isRefreshing.value = false
  }
}

onMounted(async () => {
  await fetchSession()
  isLoading.value = false
})
</script>

<template>
  <DetailPageLayout
    :title="title"
    :description="session?.flow_name || undefined"
    :icon="History"
    icon-gradient="bg-gradient-to-br from-violet-500 to-purple-600 shadow-violet-500/20"
    back-link="/chatbot/sessions"
    :breadcrumbs="breadcrumbs"
    :is-loading="isLoading"
    :is-not-found="!isLoading && !session"
    :not-found-title="t('chatbotSessions.notFound')"
  >
    <template #actions>
      <RefreshButton :refreshing="isRefreshing" :label="t('common.refresh')" @refresh="refresh" />
    </template>

    <template v-if="session">
      <SessionAnswersCard :data="session.session_data" />
      <SessionTranscriptCard :messages="session.messages || []" />
    </template>

    <template #sidebar>
      <SessionInfoCard v-if="session" :session="session" :contact-name="contactName" />
    </template>
  </DetailPageLayout>
</template>
