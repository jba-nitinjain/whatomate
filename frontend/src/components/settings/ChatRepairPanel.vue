<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { toast } from "vue-sonner";
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
import {
  chatRepairService,
  type ChatRepairCandidate,
  type ChatRepairSummary,
} from "@/services/api";
import { AlertTriangle, Loader2, RefreshCw, Wrench } from "lucide-vue-next";

const { t } = useI18n();

const isLoading = ref(false);
const isApplying = ref(false);
const summary = ref<ChatRepairSummary | null>(null);
const candidates = ref<ChatRepairCandidate[]>([]);
const selectedContactIds = ref<string[]>([]);

const safeCandidates = computed(() =>
  candidates.value.filter((candidate) => candidate.action === "move"),
);
const selectedSafeCount = computed(
  () =>
    safeCandidates.value.filter((candidate) =>
      selectedContactIds.value.includes(candidate.contact_id),
    ).length,
);

function syncSelection() {
  const safeIds = new Set(
    safeCandidates.value.map((candidate) => candidate.contact_id),
  );
  selectedContactIds.value = selectedContactIds.value.filter((id) =>
    safeIds.has(id),
  );
  if (selectedContactIds.value.length === 0 && safeIds.size > 0) {
    selectedContactIds.value = [...safeIds];
  }
}

function setCandidateSelection(contactId: string, checked: boolean) {
  selectedContactIds.value = checked
    ? Array.from(new Set([...selectedContactIds.value, contactId]))
    : selectedContactIds.value.filter((id) => id !== contactId);
}

async function loadCandidates() {
  isLoading.value = true;
  try {
    const response = await chatRepairService.preview();
    const data = (response.data as any).data || response.data;
    summary.value = data.summary;
    candidates.value = data.candidates || [];
    syncSelection();
  } catch {
    toast.error(t("settings.chatRepairPreviewFailed"));
  } finally {
    isLoading.value = false;
  }
}

async function applySafeCandidates() {
  if (!selectedContactIds.value.length) return;
  isApplying.value = true;
  try {
    const response = await chatRepairService.apply(selectedContactIds.value);
    const data = (response.data as any).data || response.data;
    toast.success(
      t("settings.chatRepairApplySuccess", {
        contacts: data.updated_contacts,
        messages: data.updated_messages,
      }),
    );
    await loadCandidates();
  } catch {
    toast.error(t("settings.chatRepairApplyFailed"));
  } finally {
    isApplying.value = false;
  }
}

onMounted(loadCandidates);
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
          class="bg-white/[0.04] border-white/[0.1] text-white/70 hover:bg-white/[0.08] hover:text-white light:bg-white light:border-gray-200 light:text-gray-700 light:hover:bg-gray-50"
          @click="loadCandidates"
          :disabled="isLoading || isApplying"
        >
          <Loader2 v-if="isLoading" class="mr-2 h-4 w-4 animate-spin" />
          <RefreshCw v-else class="mr-2 h-4 w-4" />
          {{ $t("settings.chatRepairScan") }}
        </Button>

        <Button
          size="sm"
          class="bg-amber-600 text-white hover:bg-amber-500"
          @click="applySafeCandidates"
          :disabled="!selectedSafeCount || isLoading || isApplying"
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
          @click="
            selectedContactIds = safeCandidates.map(
              (candidate) => candidate.contact_id,
            )
          "
        >
          {{ $t("common.selectAll") }}
        </button>
        <button
          type="button"
          class="hover:text-white light:hover:text-gray-900"
          @click="selectedContactIds = []"
        >
          {{ $t("common.clear") }}
        </button>
      </div>

      <div
        v-if="!candidates.length && !isLoading"
        class="rounded-lg border border-dashed border-white/[0.1] p-6 text-sm text-white/50 light:border-gray-200 light:text-gray-500"
      >
        {{ $t("settings.chatRepairEmpty") }}
      </div>

      <ChatRepairCandidateCard
        v-for="candidate in candidates"
        :key="candidate.contact_id"
        :candidate="candidate"
        :selected="selectedContactIds.includes(candidate.contact_id)"
        @toggle="
          (checked) => setCandidateSelection(candidate.contact_id, checked)
        "
      />
    </CardContent>
  </Card>
</template>
