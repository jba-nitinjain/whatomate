<script setup lang="ts">
import { onMounted } from "vue";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import ChatRepairCandidateCard from "@/components/settings/ChatRepairCandidateCard.vue";
import { useChatRepair } from "@/composables";
import { AlertTriangle, Loader2, RefreshCw, Wrench } from "lucide-vue-next";

const {
  applySafeCandidates,
  approveManualMerge,
  candidates,
  clearSafeCandidateSelection,
  isApplying,
  isLoadingCandidates,
  isScanning,
  loadCandidates,
  mergingContactId,
  safeCandidates,
  scanCandidates,
  selectAllSafeCandidates,
  selectedContactIds,
  selectedSafeCount,
  setCandidateSelection,
  summary,
} = useChatRepair();

onMounted(() => {
  void loadCandidates();
});
</script>

<template>
  <Card
    class="border-white/[0.08] bg-white/[0.02] light:border-gray-200 light:bg-white"
  >
    <CardHeader
      class="flex flex-col gap-4 md:flex-row md:items-start md:justify-between"
    >
      <div>
        <CardTitle class="text-white light:text-gray-900">{{
          $t("settings.chatRepairTitle")
        }}</CardTitle>
        <CardDescription class="text-white/40 light:text-gray-500">{{
          $t("settings.chatRepairDesc")
        }}</CardDescription>
      </div>

      <div class="flex gap-2">
        <Button
          variant="outline"
          size="sm"
          class="border-white/[0.1] bg-white/[0.04] text-white/70 hover:bg-white/[0.08] hover:text-white light:border-gray-200 light:bg-white light:text-gray-700 light:hover:bg-gray-50"
          :disabled="isLoadingCandidates || isScanning || isApplying"
          @click="scanCandidates"
        >
          <Loader2
            v-if="isLoadingCandidates || isScanning"
            class="mr-2 h-4 w-4 animate-spin"
          />
          <RefreshCw v-else class="mr-2 h-4 w-4" />
          {{ $t("settings.chatRepairScan") }}
        </Button>

        <Button
          size="sm"
          class="bg-amber-600 text-white hover:bg-amber-500"
          :disabled="
            !selectedSafeCount || isLoadingCandidates || isScanning || isApplying
          "
          @click="applySafeCandidates"
        >
          <Loader2 v-if="isApplying" class="mr-2 h-4 w-4 animate-spin" />
          <Wrench v-else class="mr-2 h-4 w-4" />
          {{ $t("settings.chatRepairApply", { count: selectedSafeCount }) }}
        </Button>
      </div>
    </CardHeader>

    <CardContent class="space-y-4">
      <Alert variant="warning">
        <AlertTriangle class="h-4 w-4" />
        <AlertTitle>{{ $t("settings.chatRepairWarningTitle") }}</AlertTitle>
        <AlertDescription>{{
          $t("settings.chatRepairWarningDesc")
        }}</AlertDescription>
      </Alert>

      <div class="grid gap-3 md:grid-cols-4">
        <div
          class="rounded-lg border border-white/[0.08] bg-black/20 p-4 light:border-gray-200 light:bg-gray-50"
        >
          <p
            class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500"
          >
            {{ $t("settings.chatRepairSafeCandidates") }}
          </p>
          <p class="mt-2 text-2xl font-semibold text-white light:text-gray-900">
            {{ summary?.auto_fixable_candidates ?? 0 }}
          </p>
        </div>
        <div
          class="rounded-lg border border-white/[0.08] bg-black/20 p-4 light:border-gray-200 light:bg-gray-50"
        >
          <p
            class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500"
          >
            {{ $t("settings.chatRepairMergeCandidates") }}
          </p>
          <p class="mt-2 text-2xl font-semibold text-white light:text-gray-900">
            {{ summary?.merge_required_candidates ?? 0 }}
          </p>
        </div>
        <div
          class="rounded-lg border border-white/[0.08] bg-black/20 p-4 light:border-gray-200 light:bg-gray-50"
        >
          <p
            class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500"
          >
            {{ $t("settings.chatRepairConflictCandidates") }}
          </p>
          <p class="mt-2 text-2xl font-semibold text-white light:text-gray-900">
            {{ summary?.conflict_candidates ?? 0 }}
          </p>
        </div>
        <div
          class="rounded-lg border border-white/[0.08] bg-black/20 p-4 light:border-gray-200 light:bg-gray-50"
        >
          <p
            class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500"
          >
            {{ $t("settings.chatRepairAffectedMessages") }}
          </p>
          <p class="mt-2 text-2xl font-semibold text-white light:text-gray-900">
            {{ summary?.affected_external_messages ?? 0 }}
          </p>
        </div>
      </div>

      <div
        v-if="safeCandidates.length"
        class="flex items-center gap-4 text-sm text-white/60 light:text-gray-600"
      >
        <button
          type="button"
          class="hover:text-white light:hover:text-gray-900"
          @click="selectAllSafeCandidates"
        >
          {{ $t("common.selectAll") }}
        </button>
        <button
          type="button"
          class="hover:text-white light:hover:text-gray-900"
          @click="clearSafeCandidateSelection"
        >
          {{ $t("common.clear") }}
        </button>
      </div>

      <div
        v-if="!candidates.length && !isLoadingCandidates"
        class="rounded-lg border border-dashed border-white/[0.1] p-6 text-sm text-white/50 light:border-gray-200 light:text-gray-500"
      >
        {{ $t("settings.chatRepairEmpty") }}
      </div>

      <ChatRepairCandidateCard
        v-for="candidate in candidates"
        :key="candidate.contact_id"
        :candidate="candidate"
        :selected="selectedContactIds.includes(candidate.contact_id)"
        :is-merging="mergingContactId === candidate.contact_id"
        @toggle="(checked) => setCandidateSelection(candidate.contact_id, checked)"
        @manual-merge="approveManualMerge(candidate.contact_id)"
      />
    </CardContent>
  </Card>
</template>
