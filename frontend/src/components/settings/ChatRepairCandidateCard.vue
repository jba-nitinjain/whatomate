<script setup lang="ts">
import { useI18n } from "vue-i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { type ChatRepairCandidate } from "@/services/api";
import { Loader2 } from "lucide-vue-next";

const props = defineProps<{
  candidate: ChatRepairCandidate;
  selected: boolean;
  isMerging?: boolean;
}>();

const emit = defineEmits<{
  toggle: [checked: boolean];
  manualMerge: [];
}>();

const { t } = useI18n();

function formatDate(value?: string) {
  if (!value) return t("settings.chatRepairNoTimestamp");
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function actionVariant(action: ChatRepairCandidate["action"]) {
  if (action === "move") return "success";
  if (action === "merge_required") return "warning";
  return "destructive";
}

function actionLabel(action: ChatRepairCandidate["action"]) {
  if (action === "move") return t("settings.chatRepairActionMove");
  if (action === "merge_required") {
    return t("settings.chatRepairActionMergeRequired");
  }
  return t("settings.chatRepairActionConflict");
}

function directionLabel(direction: string) {
  return direction === "incoming"
    ? t("settings.chatRepairDirectionIncoming")
    : t("settings.chatRepairDirectionOutgoing");
}
</script>

<template>
  <div
    class="rounded-lg border border-white/[0.08] bg-black/20 p-4 light:border-gray-200 light:bg-gray-50"
  >
    <div
      class="flex flex-col gap-3 md:flex-row md:items-start md:justify-between"
    >
      <div class="flex gap-3">
        <Checkbox
          v-if="candidate.action === 'move'"
          :checked="selected"
          @update:checked="(checked) => emit('toggle', checked === true)"
          class="mt-1"
        />
        <div>
          <div class="flex flex-wrap items-center gap-2">
            <p class="font-medium text-white light:text-gray-900">
              {{ candidate.profile_name || candidate.phone_number }}
            </p>
            <Badge :variant="actionVariant(candidate.action)">
              {{ actionLabel(candidate.action) }}
            </Badge>
          </div>
          <p class="text-sm text-white/50 light:text-gray-500">
            {{ candidate.phone_number }} |
            {{ $t("settings.chatRepairPhoneId") }}
            {{ candidate.phone_number_id }}
          </p>
        </div>
      </div>

      <p class="text-xs text-white/40 light:text-gray-500">
        {{ $t("settings.chatRepairLastMessage") }}
        {{ formatDate(candidate.last_message_at) }}
      </p>
    </div>

    <div class="mt-3 grid gap-3 text-sm md:grid-cols-2">
      <div
        class="rounded-md border border-white/[0.06] p-3 light:border-gray-200"
      >
        <p
          class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500"
        >
          {{ $t("settings.chatRepairCurrentLocation") }}
        </p>
        <p class="mt-1 text-white light:text-gray-900">
          {{ candidate.current_org_name || candidate.current_org_id }}
        </p>
        <p class="text-white/50 light:text-gray-500">
          {{ candidate.current_account }}
        </p>
      </div>

      <div
        class="rounded-md border border-white/[0.06] p-3 light:border-gray-200"
      >
        <p
          class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500"
        >
          {{ $t("settings.chatRepairTargetLocation") }}
        </p>
        <p class="mt-1 text-white light:text-gray-900">
          {{ candidate.target_org_name || candidate.target_org_id }}
        </p>
        <p class="text-white/50 light:text-gray-500">
          {{ candidate.target_account }}
        </p>
      </div>
    </div>

    <div
      class="mt-3 flex flex-wrap items-center gap-3 text-sm text-white/60 light:text-gray-600"
    >
      <span>
        {{
          $t("settings.chatRepairMessagesAffected", {
            count: candidate.affected_message_count,
          })
        }}
      </span>
      <span>{{ candidate.reason }}</span>
    </div>

    <div
      v-if="candidate.action === 'merge_required'"
      class="mt-3 flex flex-wrap items-center justify-between gap-3 rounded-md border border-amber-500/30 bg-amber-500/10 p-3 light:border-amber-200 light:bg-amber-50"
    >
      <div class="text-sm text-white/70 light:text-amber-900">
        <p class="font-medium">{{ $t("settings.chatRepairManualMergeTitle") }}</p>
        <p>
          {{
            $t("settings.chatRepairManualMergeDesc", {
              target: candidate.target_contact_id,
            })
          }}
        </p>
      </div>

      <Button
        size="sm"
        class="bg-amber-600 text-white hover:bg-amber-500"
        :disabled="isMerging"
        @click="emit('manualMerge')"
      >
        <Loader2 v-if="isMerging" class="mr-2 h-4 w-4 animate-spin" />
        <template v-else>
          {{ $t("settings.chatRepairApproveMerge") }}
        </template>
      </Button>
    </div>

    <div
      v-if="candidate.sample_messages?.length"
      class="mt-4 rounded-md border border-white/[0.06] p-3 light:border-gray-200"
    >
      <p
        class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500"
      >
        {{ $t("settings.chatRepairSampleMessages") }}
      </p>

      <div class="mt-3 space-y-2">
        <div
          v-for="(message, index) in candidate.sample_messages"
          :key="`${candidate.contact_id}-${index}-${message.created_at}`"
          class="rounded-md bg-white/[0.03] px-3 py-2 light:bg-gray-100"
        >
          <div
            class="flex flex-wrap items-center gap-2 text-xs text-white/50 light:text-gray-500"
          >
            <Badge variant="secondary">
              {{ directionLabel(message.direction) }}
            </Badge>
            <span>{{ message.message_type }}</span>
            <span>{{ formatDate(message.created_at) }}</span>
          </div>
          <p class="mt-2 text-sm text-white light:text-gray-900">
            {{ message.preview }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>
