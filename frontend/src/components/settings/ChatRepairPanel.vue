<script setup lang="ts">
import { onMounted, ref } from "vue";
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
import { AlertTriangle, Loader2, RefreshCw, Wrench, Building2 } from "lucide-vue-next";
import { orgMismatchService, type OrgMismatchRecord } from "@/services/api";
import { toast } from "vue-sonner";

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

// Org mismatch repair state
const orgMismatchRecords = ref<OrgMismatchRecord[]>([]);
const orgMismatchCount = ref<number | null>(null);
const isOrgMismatchScanning = ref(false);
const isOrgMismatchApplying = ref(false);

async function scanOrgMismatch() {
  isOrgMismatchScanning.value = true;
  try {
    const res = await orgMismatchService.preview();
    const data = (res.data as any)?.data ?? res.data;
    orgMismatchCount.value = data.count ?? 0;
    orgMismatchRecords.value = data.records ?? [];
  } catch {
    toast.error("Failed to scan for org-mismatched records");
  } finally {
    isOrgMismatchScanning.value = false;
  }
}

async function applyOrgMismatch() {
  isOrgMismatchApplying.value = true;
  try {
    const res = await orgMismatchService.apply();
    const data = (res.data as any)?.data ?? res.data;
    const fixed = data.fixed ?? 0;
    const errors: string[] = data.errors ?? [];
    if (errors.length) {
      toast.error(`Fixed ${fixed} record(s) but encountered ${errors.length} error(s)`);
    } else {
      toast.success(`Successfully fixed ${fixed} org-mismatched record(s)`);
    }
    // Re-scan after apply
    await scanOrgMismatch();
  } catch {
    toast.error("Failed to apply org mismatch fix");
  } finally {
    isOrgMismatchApplying.value = false;
  }
}

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

  <!-- Org-Mismatch Repair Panel -->
  <Card class="border-white/[0.08] bg-white/[0.02] light:border-gray-200 light:bg-white mt-6">
    <CardHeader class="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
      <div>
        <CardTitle class="text-white light:text-gray-900 flex items-center gap-2">
          <Building2 class="h-5 w-5" /> Fix Phone-ID Organisation Mismatch
        </CardTitle>
        <CardDescription class="text-white/40 light:text-gray-500">
          Finds contacts whose <code>phone_number_id</code> in metadata belongs to a different
          organisation than the contact's own org, and moves them to the correct one.
        </CardDescription>
      </div>
      <div class="flex gap-2">
        <Button
          variant="outline"
          size="sm"
          class="border-white/[0.1] bg-white/[0.04] text-white/70 hover:bg-white/[0.08] hover:text-white light:border-gray-200 light:bg-white light:text-gray-700 light:hover:bg-gray-50"
          :disabled="isOrgMismatchScanning || isOrgMismatchApplying"
          @click="scanOrgMismatch"
        >
          <Loader2 v-if="isOrgMismatchScanning" class="mr-2 h-4 w-4 animate-spin" />
          <RefreshCw v-else class="mr-2 h-4 w-4" />
          Scan
        </Button>
        <Button
          size="sm"
          class="bg-amber-600 text-white hover:bg-amber-500"
          :disabled="!orgMismatchCount || isOrgMismatchScanning || isOrgMismatchApplying"
          @click="applyOrgMismatch"
        >
          <Loader2 v-if="isOrgMismatchApplying" class="mr-2 h-4 w-4 animate-spin" />
          <Wrench v-else class="mr-2 h-4 w-4" />
          Apply Fix ({{ orgMismatchCount ?? 0 }})
        </Button>
      </div>
    </CardHeader>
    <CardContent class="space-y-3">
      <div v-if="orgMismatchCount === null" class="text-sm text-white/50 light:text-gray-500">
        Click "Scan" to check for mismatched records.
      </div>
      <div v-else-if="orgMismatchCount === 0" class="text-sm text-white/50 light:text-gray-500">
        No mismatched records found.
      </div>
      <div v-else class="space-y-2">
        <div
          v-for="rec in orgMismatchRecords"
          :key="rec.contact_id"
          class="rounded-lg border border-white/[0.08] bg-black/20 p-3 text-sm light:border-gray-200 light:bg-gray-50"
        >
          <div class="flex items-center justify-between gap-4 flex-wrap">
            <div>
              <span class="font-medium text-white light:text-gray-900">{{ rec.profile_name || rec.phone_number }}</span>
              <span class="ml-2 text-white/50 light:text-gray-500">{{ rec.phone_number }}</span>
            </div>
            <span class="text-xs text-white/40 light:text-gray-400">{{ rec.message_count }} message(s)</span>
          </div>
          <div class="mt-1 flex items-center gap-2 text-xs text-white/50 light:text-gray-500">
            <span class="text-red-400">{{ rec.current_org_name || rec.current_org_id }}</span>
            <span>→</span>
            <span class="text-green-400">{{ rec.correct_org_name || rec.correct_org_id }}</span>
            <span class="ml-auto font-mono">ID: {{ rec.phone_number_id }}</span>
          </div>
        </div>
      </div>
    </CardContent>
  </Card>
</template>
