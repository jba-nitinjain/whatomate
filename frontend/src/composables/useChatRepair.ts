import { computed, ref } from "vue";
import type { AxiosError } from "axios";
import { toast } from "vue-sonner";
import { useI18n } from "vue-i18n";
import {
  chatRepairService,
  type ApiResponseData,
  type ChatRepairApplyResult,
  type ChatRepairCandidate,
  type ChatRepairPreviewResult,
  type ChatRepairScanResult,
  type ChatRepairSummary,
} from "@/services/api";

interface ApiErrorBody {
  message?: string;
}

function isApiResponseEnvelope<T>(
  value: ApiResponseData<T>,
): value is { data: T } {
  return typeof value === "object" && value !== null && "data" in value;
}

function unwrapApiPayload<T>(value: ApiResponseData<T>): T {
  return isApiResponseEnvelope(value) ? value.data : value;
}

function getApiErrorMessage(error: unknown, fallback: string) {
  return (
    (error as AxiosError<ApiErrorBody>)?.response?.data?.message || fallback
  );
}

export function useChatRepair() {
  const { t } = useI18n();

  const isLoadingCandidates = ref(false);
  const isScanning = ref(false);
  const isApplying = ref(false);
  const mergingContactId = ref<string | null>(null);
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

  function setChatRepairState(next: ChatRepairPreviewResult | ChatRepairScanResult) {
    summary.value = next.summary;
    candidates.value = next.candidates ?? [];
    syncSelection();
  }

  async function loadCandidates() {
    isLoadingCandidates.value = true;
    try {
      const response = await chatRepairService.preview();
      setChatRepairState(unwrapApiPayload(response.data));
    } catch {
      toast.error(t("settings.chatRepairPreviewFailed"));
    } finally {
      isLoadingCandidates.value = false;
    }
  }

  async function scanCandidates() {
    isScanning.value = true;
    try {
      const response = await chatRepairService.scan();
      const data = unwrapApiPayload(response.data);
      setChatRepairState(data);

      if (
        data.auto_applied.updated_contacts > 0 ||
        data.auto_applied.updated_messages > 0
      ) {
        toast.success(
          t("settings.chatRepairScanSuccess", {
            contacts: data.auto_applied.updated_contacts,
            messages: data.auto_applied.updated_messages,
          }),
        );
      }
    } catch (error) {
      toast.error(getApiErrorMessage(error, t("settings.chatRepairScanFailed")));
    } finally {
      isScanning.value = false;
    }
  }

  async function applySafeCandidates() {
    if (!selectedContactIds.value.length) {
      return;
    }

    isApplying.value = true;
    try {
      const response = await chatRepairService.apply(selectedContactIds.value);
      const data = unwrapApiPayload<ChatRepairApplyResult>(response.data);
      toast.success(
        t("settings.chatRepairApplySuccess", {
          contacts: data.updated_contacts,
          messages: data.updated_messages,
        }),
      );
      await loadCandidates();
    } catch (error) {
      toast.error(getApiErrorMessage(error, t("settings.chatRepairApplyFailed")));
    } finally {
      isApplying.value = false;
    }
  }

  async function approveManualMerge(contactId: string) {
    mergingContactId.value = contactId;
    try {
      const response = await chatRepairService.apply([], [contactId]);
      const data = unwrapApiPayload<ChatRepairApplyResult>(response.data);
      toast.success(
        t("settings.chatRepairManualMergeSuccess", {
          contacts: data.updated_contacts,
          messages: data.updated_messages,
        }),
      );
      await loadCandidates();
    } catch (error) {
      toast.error(
        getApiErrorMessage(error, t("settings.chatRepairManualMergeFailed")),
      );
    } finally {
      mergingContactId.value = null;
    }
  }

  function setCandidateSelection(contactId: string, checked: boolean) {
    selectedContactIds.value = checked
      ? Array.from(new Set([...selectedContactIds.value, contactId]))
      : selectedContactIds.value.filter((id) => id !== contactId);
  }

  function selectAllSafeCandidates() {
    selectedContactIds.value = safeCandidates.value.map(
      (candidate) => candidate.contact_id,
    );
  }

  function clearSafeCandidateSelection() {
    selectedContactIds.value = [];
  }

  return {
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
  };
}
