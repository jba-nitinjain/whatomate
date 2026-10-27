<script setup lang="ts">
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Building2 } from 'lucide-vue-next'
import type { Organization } from '@/services/api'
import { formatDate } from '@/lib/utils'

defineProps<{
  organization: Organization | null
  memberCount: number
  loading: boolean
}>()
</script>

<template>
  <Card data-testid="current-organization">
    <CardContent class="p-5">
      <div class="flex flex-wrap items-center gap-4">
        <div class="h-11 w-11 shrink-0 rounded-lg bg-gradient-to-br from-teal-500 to-emerald-600 flex items-center justify-center shadow-lg shadow-teal-500/20">
          <Building2 class="h-5 w-5 text-white" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-xs uppercase tracking-wide text-muted-foreground">{{ $t('orgMembers.currentOrganization') }}</p>
          <Skeleton v-if="loading && !organization" class="mt-1 h-6 w-48" />
          <p v-else class="text-lg font-semibold truncate">{{ organization?.name || '—' }}</p>
        </div>
        <dl class="flex flex-wrap gap-x-8 gap-y-2 text-sm">
          <div v-if="organization?.slug">
            <dt class="text-muted-foreground">{{ $t('orgMembers.slug') }}</dt>
            <dd class="font-mono">{{ organization.slug }}</dd>
          </div>
          <div v-if="organization?.created_at">
            <dt class="text-muted-foreground">{{ $t('orgMembers.createdOn') }}</dt>
            <dd>{{ formatDate(organization.created_at) }}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground">{{ $t('orgMembers.totalMembers') }}</dt>
            <dd class="font-semibold">{{ memberCount }}</dd>
          </div>
        </dl>
      </div>
    </CardContent>
  </Card>
</template>
