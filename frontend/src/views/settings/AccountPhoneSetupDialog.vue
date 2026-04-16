<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { toast } from "vue-sonner";
import { api } from "@/services/api";
import { getErrorMessage } from "@/lib/api-utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Dialog,
  DialogHeader,
  DialogDescription,
  DialogScrollContent,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  AlertCircle,
  CheckCircle2,
  Loader2,
  RefreshCw,
  ShieldCheck,
} from "lucide-vue-next";

interface Props {
  open: boolean;
  accountId: string | null;
  accountName: string;
}

interface PhoneStatus {
  display_phone_number: string;
  verified_name: string;
  code_verification_status: string;
  account_mode: string;
  quality_rating: string;
  messaging_limit_tier: string;
  is_test_number: boolean;
  warning?: string;
  two_step_disable_supported: boolean;
}

const props = defineProps<Props>();
const emit = defineEmits(["update:open"]);

const { t } = useI18n();

const dialogOpen = computed({
  get: () => props.open,
  set: (value: boolean) => emit("update:open", value),
});

const status = ref<PhoneStatus | null>(null);
const isLoadingStatus = ref(false);
const isRequestingCode = ref(false);
const isVerifyingCode = ref(false);
const isRegisteringPhone = ref(false);
const isUpdatingPin = ref(false);

const requestCodeForm = ref({
  code_method: "SMS",
  language: "en_US",
});

const verifyCodeForm = ref({
  code: "",
});

const registerForm = ref({
  pin: "",
  backup_password: "",
  backup_data: "",
});

const twoStepForm = ref({
  pin: "",
});

watch(
  [() => props.open, () => props.accountId],
  async ([isOpen, accountId]) => {
    if (!isOpen || !accountId) {
      return;
    }
    await fetchStatus();
  },
);

function hasSixDigitPin(pin: string) {
  return /^\d{6}$/.test(pin.trim());
}

function statusBadgeClass(statusValue?: string) {
  switch (statusValue) {
    case "VERIFIED":
      return "border-green-500/40 bg-green-950/60 text-green-200 light:border-green-500/40 light:bg-green-50 light:text-green-700";
    case "NOT_VERIFIED":
      return "border-amber-500/40 bg-amber-950/60 text-amber-200 light:border-amber-500/40 light:bg-amber-50 light:text-amber-700";
    case "EXPIRED":
      return "border-red-500/40 bg-red-950/60 text-red-200 light:border-red-500/40 light:bg-red-50 light:text-red-700";
    default:
      return "border-white/10 bg-white/5 text-white/80 light:border-gray-200 light:bg-gray-100 light:text-gray-700";
  }
}

async function fetchStatus(showSuccessToast = false) {
  if (!props.accountId) return;

  isLoadingStatus.value = true;
  try {
    const response = await api.get(`/accounts/${props.accountId}/phone-status`);
    status.value = response.data.data;
    if (showSuccessToast) {
      toast.success(t("accounts.phoneStatusRefreshed"));
    }
  } catch (error: any) {
    status.value = null;
    toast.error(getErrorMessage(error, t("accounts.statusUnavailable")));
  } finally {
    isLoadingStatus.value = false;
  }
}

async function requestCode() {
  if (!props.accountId) return;

  isRequestingCode.value = true;
  try {
    await api.post(`/accounts/${props.accountId}/request-code`, {
      code_method: requestCodeForm.value.code_method,
      language: requestCodeForm.value.language.trim() || "en_US",
    });
    toast.success(t("accounts.requestCodeSuccess"));
  } catch (error: any) {
    toast.error(getErrorMessage(error, t("accounts.requestCodeFailed")));
  } finally {
    isRequestingCode.value = false;
  }
}

async function verifyCode() {
  if (!props.accountId) return;

  const code = verifyCodeForm.value.code.trim();
  if (!code) {
    toast.error(t("accounts.verificationCodeRequired"));
    return;
  }

  isVerifyingCode.value = true;
  try {
    await api.post(`/accounts/${props.accountId}/verify-code`, { code });
    verifyCodeForm.value.code = "";
    toast.success(t("accounts.verifyCodeSuccess"));
    await fetchStatus();
  } catch (error: any) {
    toast.error(getErrorMessage(error, t("accounts.verifyCodeFailed")));
  } finally {
    isVerifyingCode.value = false;
  }
}

async function registerPhone() {
  if (!props.accountId) return;

  if (!hasSixDigitPin(registerForm.value.pin)) {
    toast.error(t("accounts.pinMustBeSixDigits"));
    return;
  }

  const hasBackupPassword = registerForm.value.backup_password.trim() !== "";
  const hasBackupData = registerForm.value.backup_data.trim() !== "";
  if (hasBackupPassword !== hasBackupData) {
    toast.error(t("accounts.backupFieldsRequiredTogether"));
    return;
  }

  isRegisteringPhone.value = true;
  try {
    await api.post(`/accounts/${props.accountId}/register-phone`, {
      pin: registerForm.value.pin.trim(),
      backup_password: registerForm.value.backup_password.trim(),
      backup_data: registerForm.value.backup_data.trim(),
    });
    registerForm.value.pin = "";
    registerForm.value.backup_password = "";
    registerForm.value.backup_data = "";
    toast.success(t("accounts.registerPhoneSuccess"));
    await fetchStatus();
  } catch (error: any) {
    toast.error(getErrorMessage(error, t("accounts.registerPhoneFailed")));
  } finally {
    isRegisteringPhone.value = false;
  }
}

async function updateTwoStepPin() {
  if (!props.accountId) return;

  if (!hasSixDigitPin(twoStepForm.value.pin)) {
    toast.error(t("accounts.pinMustBeSixDigits"));
    return;
  }

  isUpdatingPin.value = true;
  try {
    await api.post(`/accounts/${props.accountId}/two-step-verification`, {
      pin: twoStepForm.value.pin.trim(),
    });
    twoStepForm.value.pin = "";
    toast.success(t("accounts.updatePinSuccess"));
    await fetchStatus();
  } catch (error: any) {
    toast.error(getErrorMessage(error, t("accounts.updatePinFailed")));
  } finally {
    isUpdatingPin.value = false;
  }
}
</script>

<template>
  <Dialog v-model:open="dialogOpen">
    <DialogScrollContent class="max-w-4xl">
      <DialogHeader>
        <DialogTitle class="flex items-center gap-2">
          <ShieldCheck class="h-5 w-5 text-emerald-500" />
          {{ $t("accounts.phoneSetup") }}: {{ accountName }}
        </DialogTitle>
        <DialogDescription>
          {{ $t("accounts.phoneSetupDescription") }}
        </DialogDescription>
      </DialogHeader>

      <div class="space-y-6">
        <Alert variant="info">
          <AlertCircle class="h-4 w-4" />
          <AlertTitle>{{ $t("accounts.metaPhoneFlow") }}</AlertTitle>
          <AlertDescription>
            {{ $t("accounts.metaPhoneFlowDesc") }}
          </AlertDescription>
        </Alert>

        <section
          class="rounded-xl border border-white/[0.08] bg-white/[0.02] p-5 light:border-gray-200 light:bg-gray-50"
        >
          <div
            class="flex flex-col gap-4 md:flex-row md:items-start md:justify-between"
          >
            <div>
              <h3 class="text-sm font-semibold text-white light:text-gray-900">
                {{ $t("accounts.phoneStatus") }}
              </h3>
              <p class="mt-1 text-sm text-white/60 light:text-gray-600">
                {{ $t("accounts.phoneStatusDescription") }}
              </p>
            </div>
            <Button
              variant="outline"
              size="sm"
              :disabled="isLoadingStatus"
              @click="fetchStatus(true)"
            >
              <Loader2
                v-if="isLoadingStatus"
                class="mr-2 h-4 w-4 animate-spin"
              />
              <RefreshCw v-else class="mr-2 h-4 w-4" />
              {{ $t("accounts.refreshStatus") }}
            </Button>
          </div>

          <div
            v-if="status"
            class="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-3"
          >
            <div
              class="rounded-lg border border-white/[0.08] bg-black/20 p-3 light:border-gray-200 light:bg-white"
            >
              <p
                class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500"
              >
                {{ $t("accounts.displayPhoneNumber") }}
              </p>
              <p class="mt-1 font-medium text-white light:text-gray-900">
                {{ status.display_phone_number || "N/A" }}
              </p>
            </div>
            <div
              class="rounded-lg border border-white/[0.08] bg-black/20 p-3 light:border-gray-200 light:bg-white"
            >
              <p
                class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500"
              >
                {{ $t("accounts.verifiedName") }}
              </p>
              <p class="mt-1 font-medium text-white light:text-gray-900">
                {{ status.verified_name || "N/A" }}
              </p>
            </div>
            <div
              class="rounded-lg border border-white/[0.08] bg-black/20 p-3 light:border-gray-200 light:bg-white"
            >
              <p
                class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500"
              >
                {{ $t("accounts.verificationStatus") }}
              </p>
              <div class="mt-2">
                <Badge
                  variant="outline"
                  :class="statusBadgeClass(status.code_verification_status)"
                >
                  {{ status.code_verification_status || "UNKNOWN" }}
                </Badge>
              </div>
            </div>
            <div
              class="rounded-lg border border-white/[0.08] bg-black/20 p-3 light:border-gray-200 light:bg-white"
            >
              <p
                class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500"
              >
                {{ $t("accounts.accountMode") }}
              </p>
              <p class="mt-1 font-medium text-white light:text-gray-900">
                {{ status.account_mode || "N/A" }}
              </p>
            </div>
            <div
              class="rounded-lg border border-white/[0.08] bg-black/20 p-3 light:border-gray-200 light:bg-white"
            >
              <p
                class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500"
              >
                {{ $t("accounts.qualityRating") }}
              </p>
              <p class="mt-1 font-medium text-white light:text-gray-900">
                {{ status.quality_rating || "N/A" }}
              </p>
            </div>
            <div
              class="rounded-lg border border-white/[0.08] bg-black/20 p-3 light:border-gray-200 light:bg-white"
            >
              <p
                class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500"
              >
                {{ $t("accounts.messagingLimitTier") }}
              </p>
              <p class="mt-1 font-medium text-white light:text-gray-900">
                {{ status.messaging_limit_tier || "N/A" }}
              </p>
            </div>
          </div>

          <p
            v-else-if="!isLoadingStatus"
            class="mt-4 text-sm text-white/60 light:text-gray-600"
          >
            {{ $t("accounts.statusUnavailable") }}
          </p>

          <Alert v-if="status?.warning" variant="warning" class="mt-4">
            <AlertCircle class="h-4 w-4" />
            <AlertTitle>{{ $t("common.warning") }}</AlertTitle>
            <AlertDescription>{{ status.warning }}</AlertDescription>
          </Alert>
        </section>

        <section
          class="rounded-xl border border-white/[0.08] bg-white/[0.02] p-5 light:border-gray-200 light:bg-gray-50"
        >
          <div>
            <h3 class="text-sm font-semibold text-white light:text-gray-900">
              {{ $t("accounts.verificationFlow") }}
            </h3>
            <p class="mt-1 text-sm text-white/60 light:text-gray-600">
              {{ $t("accounts.verificationFlowDescription") }}
            </p>
          </div>

          <div class="mt-4 grid gap-6 lg:grid-cols-2">
            <div
              class="space-y-4 rounded-lg border border-white/[0.08] bg-black/20 p-4 light:border-gray-200 light:bg-white"
            >
              <div>
                <h4 class="font-medium text-white light:text-gray-900">
                  {{ $t("accounts.requestCode") }}
                </h4>
                <p class="mt-1 text-sm text-white/60 light:text-gray-600">
                  {{ $t("accounts.requestCodeDescription") }}
                </p>
              </div>

              <div class="space-y-2">
                <Label>{{ $t("accounts.verificationMethod") }}</Label>
                <Select v-model="requestCodeForm.code_method">
                  <SelectTrigger>
                    <SelectValue
                      :placeholder="$t('accounts.verificationMethod')"
                    />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="SMS">SMS</SelectItem>
                    <SelectItem value="VOICE">VOICE</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div class="space-y-2">
                <Label for="verification-language">{{
                  $t("accounts.verificationLanguage")
                }}</Label>
                <Input
                  id="verification-language"
                  v-model="requestCodeForm.language"
                  placeholder="en_US"
                />
              </div>

              <Button :disabled="isRequestingCode" @click="requestCode">
                <Loader2
                  v-if="isRequestingCode"
                  class="mr-2 h-4 w-4 animate-spin"
                />
                {{ $t("accounts.requestCode") }}
              </Button>
            </div>

            <div
              class="space-y-4 rounded-lg border border-white/[0.08] bg-black/20 p-4 light:border-gray-200 light:bg-white"
            >
              <div>
                <h4 class="font-medium text-white light:text-gray-900">
                  {{ $t("accounts.verifyCode") }}
                </h4>
                <p class="mt-1 text-sm text-white/60 light:text-gray-600">
                  {{ $t("accounts.verifyCodeDescription") }}
                </p>
              </div>

              <div class="space-y-2">
                <Label for="verification-code">{{
                  $t("accounts.verificationCode")
                }}</Label>
                <Input
                  id="verification-code"
                  v-model="verifyCodeForm.code"
                  :placeholder="$t('accounts.verificationCodePlaceholder')"
                />
              </div>

              <Button :disabled="isVerifyingCode" @click="verifyCode">
                <Loader2
                  v-if="isVerifyingCode"
                  class="mr-2 h-4 w-4 animate-spin"
                />
                <CheckCircle2 v-else class="mr-2 h-4 w-4" />
                {{ $t("accounts.verifyCode") }}
              </Button>
            </div>
          </div>
        </section>

        <section
          class="rounded-xl border border-white/[0.08] bg-white/[0.02] p-5 light:border-gray-200 light:bg-gray-50"
        >
          <div>
            <h3 class="text-sm font-semibold text-white light:text-gray-900">
              {{ $t("accounts.registerPhone") }}
            </h3>
            <p class="mt-1 text-sm text-white/60 light:text-gray-600">
              {{ $t("accounts.registerPhoneDescription") }}
            </p>
          </div>

          <div class="mt-4 grid gap-4 lg:grid-cols-2">
            <div class="space-y-2">
              <Label for="registration-pin">{{
                $t("accounts.registrationPin")
              }}</Label>
              <Input
                id="registration-pin"
                v-model="registerForm.pin"
                type="password"
                inputmode="numeric"
                maxlength="6"
                :placeholder="$t('accounts.pinPlaceholder')"
              />
            </div>

            <div class="space-y-2">
              <Label for="backup-password">{{
                $t("accounts.backupPassword")
              }}</Label>
              <Input
                id="backup-password"
                v-model="registerForm.backup_password"
                type="password"
                :placeholder="$t('accounts.backupPasswordPlaceholder')"
              />
            </div>

            <div class="space-y-2 lg:col-span-2">
              <Label for="backup-data">{{ $t("accounts.backupData") }}</Label>
              <Textarea
                id="backup-data"
                v-model="registerForm.backup_data"
                :rows="4"
                :placeholder="$t('accounts.backupDataPlaceholder')"
              />
              <p class="text-xs text-white/45 light:text-gray-500">
                {{ $t("accounts.backupDataHint") }}
              </p>
            </div>
          </div>

          <Alert variant="info" class="mt-4">
            <AlertCircle class="h-4 w-4" />
            <AlertTitle>{{ $t("accounts.registerPhoneNoteTitle") }}</AlertTitle>
            <AlertDescription>{{
              $t("accounts.registerPhoneNote")
            }}</AlertDescription>
          </Alert>

          <div class="mt-4">
            <Button :disabled="isRegisteringPhone" @click="registerPhone">
              <Loader2
                v-if="isRegisteringPhone"
                class="mr-2 h-4 w-4 animate-spin"
              />
              {{ $t("accounts.registerPhone") }}
            </Button>
          </div>
        </section>

        <Separator />

        <section
          class="rounded-xl border border-white/[0.08] bg-white/[0.02] p-5 light:border-gray-200 light:bg-gray-50"
        >
          <div>
            <h3 class="text-sm font-semibold text-white light:text-gray-900">
              {{ $t("accounts.twoStepVerification") }}
            </h3>
            <p class="mt-1 text-sm text-white/60 light:text-gray-600">
              {{ $t("accounts.twoStepVerificationDescription") }}
            </p>
          </div>

          <div
            class="mt-4 grid gap-4 lg:grid-cols-[minmax(0,280px)_auto] lg:items-end"
          >
            <div class="space-y-2">
              <Label for="two-step-pin">{{ $t("accounts.twoStepPin") }}</Label>
              <Input
                id="two-step-pin"
                v-model="twoStepForm.pin"
                type="password"
                inputmode="numeric"
                maxlength="6"
                :placeholder="$t('accounts.pinPlaceholder')"
              />
            </div>

            <div>
              <Button :disabled="isUpdatingPin" @click="updateTwoStepPin">
                <Loader2
                  v-if="isUpdatingPin"
                  class="mr-2 h-4 w-4 animate-spin"
                />
                {{ $t("accounts.updatePin") }}
              </Button>
            </div>
          </div>

          <Alert variant="warning" class="mt-4">
            <AlertCircle class="h-4 w-4" />
            <AlertTitle>{{
              $t("accounts.twoStepDisableUnavailableTitle")
            }}</AlertTitle>
            <AlertDescription>{{
              $t("accounts.twoStepDisableUnavailable")
            }}</AlertDescription>
          </Alert>
        </section>
      </div>
    </DialogScrollContent>
  </Dialog>
</template>
