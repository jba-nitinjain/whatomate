<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { toast } from "vue-sonner";
import {
  onboardingService,
  unwrapApiPayload,
  type WhatsAppOnboardingSession,
} from "@/services/api";
import { getErrorMessage } from "@/lib/api-utils";
import { useAuthStore } from "@/stores/auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Dialog,
  DialogHeader,
  DialogDescription,
  DialogScrollContent,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  AlertCircle,
  ClipboardCheck,
  Link2,
  Loader2,
  RefreshCw,
  Rocket,
  ShieldCheck,
  TestTube2,
} from "lucide-vue-next";

interface Props {
  open: boolean;
}

declare global {
  interface Window {
    FB?: any;
    fbAsyncInit?: () => void;
  }
}

const props = defineProps<Props>();
const emit = defineEmits(["update:open", "accounts-changed"]);
const authStore = useAuthStore();

const dialogOpen = computed({
  get: () => props.open,
  set: (value: boolean) => emit("update:open", value),
});

const session = ref<WhatsAppOnboardingSession | null>(null);
const isLoadingSession = ref(false);
const isWorking = ref(false);
const mode = ref<"embedded_signup" | "manual_import">("embedded_signup");
const embeddedSessionInfo = ref<Record<string, any>>({});
const facebookSdkPromise = ref<Promise<any> | null>(null);

const embeddedSignupForm = ref({
  account_name: "",
  code: "",
  access_token: "",
  phone_id: "",
  business_id: "",
  app_id: "",
});

const manualImportForm = ref({
  account_name: "",
  app_id: "",
  app_secret: "",
  phone_id: "",
  business_id: "",
  access_token: "",
  api_version: "v21.0",
});

const phoneSetupForm = ref({
  code_method: "SMS" as "SMS" | "VOICE",
  language: "en_US",
  code: "",
  pin: "",
  backup_password: "",
  backup_data: "",
});

const currentOrgId = computed(
  () =>
    localStorage.getItem("selected_organization_id") ||
    authStore.user?.organization_id ||
    "default",
);

const sessionStorageKey = computed(
  () => `whatomate:onboarding-session:${currentOrgId.value}`,
);

const preflightDetails = computed(
  () => session.value?.step_state?.preflight?.details ?? {},
);
const phoneStepDetails = computed(
  () => session.value?.step_state?.phone_setup?.details ?? {},
);
const webhookStepDetails = computed(
  () => session.value?.step_state?.webhooks?.details ?? {},
);
const hasImportedAccount = computed(() => !!session.value?.account_id);

watch(
  () => props.open,
  async (isOpen) => {
    if (!isOpen) return;
    await ensureSession();
  },
);

function setSession(next: WhatsAppOnboardingSession) {
  session.value = next;
  mode.value = next.mode;
  localStorage.setItem(sessionStorageKey.value, next.id);
  embeddedSignupForm.value.account_name =
    next.account_name || embeddedSignupForm.value.account_name;
  embeddedSignupForm.value.phone_id =
    next.phone_id || embeddedSignupForm.value.phone_id;
  embeddedSignupForm.value.business_id =
    next.business_id || embeddedSignupForm.value.business_id;
  embeddedSignupForm.value.app_id =
    next.app_id || String(preflightDetails.value.meta_app_id || "");
  manualImportForm.value.account_name =
    next.account_name || manualImportForm.value.account_name;
  manualImportForm.value.app_id =
    next.app_id || String(preflightDetails.value.meta_app_id || "");
  manualImportForm.value.phone_id =
    next.phone_id || manualImportForm.value.phone_id;
  manualImportForm.value.business_id =
    next.business_id || manualImportForm.value.business_id;
  manualImportForm.value.api_version =
    next.api_version || manualImportForm.value.api_version;
}

async function ensureSession(forceNew = false) {
  isLoadingSession.value = true;
  try {
    const storedId = forceNew ? "" : localStorage.getItem(sessionStorageKey.value);
    if (storedId) {
      try {
        const response = await onboardingService.getSession(storedId);
        setSession(unwrapApiPayload(response.data).session);
        return;
      } catch {
        localStorage.removeItem(sessionStorageKey.value);
      }
    }
    const response = await onboardingService.createSession({ mode: mode.value });
    setSession(unwrapApiPayload(response.data).session);
  } catch (error: any) {
    toast.error(getErrorMessage(error, "Failed to initialize onboarding."));
  } finally {
    isLoadingSession.value = false;
  }
}

async function refreshSession() {
  if (!session.value) return;
  isLoadingSession.value = true;
  try {
    const response = await onboardingService.getSession(session.value.id);
    setSession(unwrapApiPayload(response.data).session);
  } catch (error: any) {
    toast.error(getErrorMessage(error, "Failed to refresh onboarding session."));
  } finally {
    isLoadingSession.value = false;
  }
}

function statusVariant(status?: string) {
  switch (status) {
    case "completed":
    case "ready":
      return "border-green-500/40 bg-green-950/60 text-green-200 light:border-green-500/40 light:bg-green-50 light:text-green-700";
    case "waiting_on_meta":
      return "border-blue-500/40 bg-blue-950/60 text-blue-200 light:border-blue-500/40 light:bg-blue-50 light:text-blue-700";
    case "action_required":
    case "failed":
      return "border-red-500/40 bg-red-950/60 text-red-200 light:border-red-500/40 light:bg-red-50 light:text-red-700";
    case "in_progress":
      return "border-amber-500/40 bg-amber-950/60 text-amber-200 light:border-amber-500/40 light:bg-amber-50 light:text-amber-700";
    default:
      return "border-white/10 bg-white/5 text-white/80 light:border-gray-200 light:bg-gray-100 light:text-gray-700";
  }
}

function stepStatus(step: string) {
  return session.value?.step_state?.[step]?.status || "pending";
}

function stepSummary(step: string) {
  return session.value?.step_state?.[step]?.summary || "";
}

async function withSessionMutation(
  action: () => Promise<WhatsAppOnboardingSession>,
  successMessage?: string,
) {
  isWorking.value = true;
  try {
    const next = await action();
    setSession(next);
    if (successMessage) toast.success(successMessage);
    if (next.account_id) emit("accounts-changed");
  } catch (error: any) {
    toast.error(getErrorMessage(error, "Onboarding action failed."));
  } finally {
    isWorking.value = false;
  }
}

async function importManual() {
  if (!session.value) return;
  if (
    !manualImportForm.value.phone_id.trim() ||
    !manualImportForm.value.business_id.trim() ||
    !manualImportForm.value.access_token.trim()
  ) {
    toast.error("Phone ID, business ID, and access token are required.");
    return;
  }

  await withSessionMutation(async () => {
    const response = await onboardingService.manualImport(session.value!.id, {
      ...manualImportForm.value,
      account_name: manualImportForm.value.account_name.trim(),
      phone_id: manualImportForm.value.phone_id.trim(),
      business_id: manualImportForm.value.business_id.trim(),
      access_token: manualImportForm.value.access_token.trim(),
      app_id: manualImportForm.value.app_id.trim(),
      app_secret: manualImportForm.value.app_secret.trim(),
      api_version: manualImportForm.value.api_version.trim(),
    });
    return unwrapApiPayload(response.data).session;
  }, "Meta assets imported.");
}

async function saveEmbeddedSignupResult(codeFromSdk?: string) {
  if (!session.value) return;

  const payload = {
    account_name: embeddedSignupForm.value.account_name.trim(),
    code: codeFromSdk || embeddedSignupForm.value.code.trim(),
    access_token: embeddedSignupForm.value.access_token.trim(),
    phone_id: embeddedSignupForm.value.phone_id.trim(),
    business_id: embeddedSignupForm.value.business_id.trim(),
    app_id:
      embeddedSignupForm.value.app_id.trim() ||
      String(preflightDetails.value.meta_app_id || ""),
    metadata: embeddedSessionInfo.value,
  };

  if (!payload.phone_id || !payload.business_id) {
    toast.error("Phone ID and business ID are required.");
    return;
  }
  if (!payload.code && !payload.access_token) {
    toast.error("Enter either the Meta authorization code or a business access token.");
    return;
  }

  await withSessionMutation(async () => {
    const response = await onboardingService.completeEmbeddedSignup(
      session.value!.id,
      payload,
    );
    return unwrapApiPayload(response.data).session;
  }, "Embedded signup assets saved.");
}

function parseEmbeddedMessage(data: any) {
  if (!data) return null;
  if (typeof data === "string") {
    try {
      return JSON.parse(data);
    } catch {
      return null;
    }
  }
  return data;
}

function isFacebookOrigin(origin: string) {
  return (
    origin === "https://www.facebook.com" ||
    origin === "https://web.facebook.com" ||
    origin.endsWith(".facebook.com")
  );
}

function syncEmbeddedFieldsFromPayload(payload: any) {
  const data = payload?.data || payload || {};
  const phoneId =
    data.phone_number_id ||
    data.phone_id ||
    data.business_phone_number_id ||
    "";
  const businessId = data.waba_id || data.business_id || "";
  if (phoneId) {
    embeddedSignupForm.value.phone_id = String(phoneId);
  }
  if (businessId) {
    embeddedSignupForm.value.business_id = String(businessId);
  }
}

async function ensureFacebookSdk(appId: string, version: string) {
  if (window.FB) {
    window.FB.init({ appId, cookie: true, xfbml: false, version });
    return window.FB;
  }

  if (!facebookSdkPromise.value) {
    facebookSdkPromise.value = new Promise((resolve, reject) => {
      window.fbAsyncInit = () => {
        if (!window.FB) {
          reject(new Error("Facebook SDK did not load."));
          return;
        }
        window.FB.init({ appId, cookie: true, xfbml: false, version });
        resolve(window.FB);
      };

      if (document.getElementById("facebook-jssdk")) {
        return;
      }

      const script = document.createElement("script");
      script.id = "facebook-jssdk";
      script.async = true;
      script.defer = true;
      script.src = "https://connect.facebook.net/en_US/sdk.js";
      script.onerror = () => reject(new Error("Failed to load Facebook SDK."));
      document.head.appendChild(script);
    });
  }

  return facebookSdkPromise.value as Promise<any>;
}

async function launchEmbeddedSignup() {
  if (!session.value) return;
  const appId = String(preflightDetails.value.meta_app_id || "");
  const configId = String(preflightDetails.value.embedded_config_id || "");
  const scopes = Array.isArray(preflightDetails.value.required_scopes)
    ? preflightDetails.value.required_scopes.join(",")
    : "";
  const version = String(
    preflightDetails.value.graph_api_version || session.value.api_version || "v21.0",
  );

  if (!appId || !configId) {
    toast.error(
      "Embedded signup is not ready. A super admin must save the Meta app configuration first.",
    );
    return;
  }

  embeddedSessionInfo.value = {};
  const messageHandler = (event: MessageEvent) => {
    if (!isFacebookOrigin(event.origin)) return;
    const payload = parseEmbeddedMessage(event.data);
    if (!payload) return;
    embeddedSessionInfo.value = payload;
    syncEmbeddedFieldsFromPayload(payload);
  };
  window.addEventListener("message", messageHandler);

  try {
    const FB = await ensureFacebookSdk(appId, version);
    const loginOptions: Record<string, any> = {
      config_id: configId,
      response_type: "code",
      override_default_response_type: true,
      extras: { sessionInfoVersion: 3 },
    };
    if (scopes) {
      loginOptions.scope = scopes;
    }
    FB.login(
      async (response: any) => {
        window.removeEventListener("message", messageHandler);
        const code = response?.authResponse?.code;
        if (!code) {
          toast.error(
            "Meta did not return an authorization code. Use the fallback fields below if the popup completed.",
          );
          return;
        }
        await saveEmbeddedSignupResult(code);
      },
      loginOptions,
    );
  } catch (error: any) {
    window.removeEventListener("message", messageHandler);
    toast.error(getErrorMessage(error, "Failed to launch Meta embedded signup."));
  }
}

async function requestCode() {
  if (!session.value) return;
  await withSessionMutation(async () => {
    const response = await onboardingService.requestCode(session.value!.id, {
      code_method: phoneSetupForm.value.code_method,
      language: phoneSetupForm.value.language.trim() || "en_US",
    });
    return unwrapApiPayload(response.data).session;
  }, "Verification code requested.");
}

async function verifyCode() {
  if (!session.value) return;
  if (!phoneSetupForm.value.code.trim()) {
    toast.error("Enter the Meta verification code.");
    return;
  }
  await withSessionMutation(async () => {
    const response = await onboardingService.verifyCode(session.value!.id, {
      code: phoneSetupForm.value.code.trim(),
    });
    phoneSetupForm.value.code = "";
    return unwrapApiPayload(response.data).session;
  }, "Verification code confirmed.");
}

async function registerPhone() {
  if (!session.value) return;
  if (!/^\d{6}$/.test(phoneSetupForm.value.pin.trim())) {
    toast.error("PIN must be exactly 6 digits.");
    return;
  }
  await withSessionMutation(async () => {
    const response = await onboardingService.registerPhone(session.value!.id, {
      pin: phoneSetupForm.value.pin.trim(),
      backup_password: phoneSetupForm.value.backup_password.trim(),
      backup_data: phoneSetupForm.value.backup_data.trim(),
    });
    phoneSetupForm.value.pin = "";
    return unwrapApiPayload(response.data).session;
  }, "Phone number registered.");
}

async function validateWebhook() {
  if (!session.value) return;
  await withSessionMutation(async () => {
    const response = await onboardingService.validateWebhook(session.value!.id);
    return unwrapApiPayload(response.data).session;
  }, "Webhook callback validated.");
}

async function subscribeWebhooks() {
  if (!session.value) return;
  await withSessionMutation(async () => {
    const response = await onboardingService.subscribeWebhooks(session.value!.id);
    return unwrapApiPayload(response.data).session;
  }, "Webhook subscription updated.");
}

async function finalizeOnboarding() {
  if (!session.value) return;
  await withSessionMutation(async () => {
    const response = await onboardingService.finalize(session.value!.id);
    return unwrapApiPayload(response.data).session;
  }, "Final readiness check completed.");
}
</script>

<template>
  <Dialog v-model:open="dialogOpen">
    <DialogScrollContent class="max-w-5xl">
      <DialogHeader>
        <DialogTitle class="flex items-center gap-2">
          <Rocket class="h-5 w-5 text-emerald-500" />
          Hybrid WhatsApp Onboarding
        </DialogTitle>
        <DialogDescription>
          This wizard persists progress on the server so you can resume later.
        </DialogDescription>
      </DialogHeader>

      <div class="space-y-6">
        <div class="flex items-center justify-between gap-3">
          <Button variant="outline" size="sm" :disabled="isLoadingSession" @click="refreshSession">
            <RefreshCw class="mr-2 h-4 w-4" />
            Refresh session
          </Button>
          <Button variant="ghost" size="sm" :disabled="isLoadingSession || isWorking" @click="ensureSession(true)">
            Start fresh
          </Button>
        </div>

        <div v-if="isLoadingSession" class="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 class="h-4 w-4 animate-spin" />
          Loading onboarding session...
        </div>

        <template v-else-if="session">
          <Alert v-if="stepStatus('preflight') === 'action_required'" variant="warning">
            <AlertCircle class="h-4 w-4" />
            <AlertTitle>Preflight blocked</AlertTitle>
            <AlertDescription>{{ stepSummary("preflight") }}</AlertDescription>
          </Alert>

          <section class="rounded-xl border border-white/[0.08] bg-white/[0.02] p-5 light:border-gray-200 light:bg-gray-50">
            <div class="flex items-start justify-between gap-4">
              <div>
                <h3 class="text-sm font-semibold text-white light:text-gray-900">1. Preflight</h3>
                <p class="mt-1 text-sm text-white/60 light:text-gray-600">
                  Confirm that the Meta platform setup exists before org-level onboarding starts.
                </p>
              </div>
              <Badge variant="outline" :class="statusVariant(stepStatus('preflight'))">
                {{ stepStatus("preflight") }}
              </Badge>
            </div>
            <p v-if="stepSummary('preflight')" class="mt-3 text-sm text-white/70 light:text-gray-700">
              {{ stepSummary("preflight") }}
            </p>
            <div class="mt-4 grid gap-3 md:grid-cols-2">
              <div class="rounded-lg border border-white/[0.08] bg-black/20 p-3 light:border-gray-200 light:bg-white">
                <p class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500">Callback URL</p>
                <p class="mt-1 break-all font-medium text-white light:text-gray-900">
                  {{ preflightDetails.callback_url || "Not configured" }}
                </p>
              </div>
              <div class="rounded-lg border border-white/[0.08] bg-black/20 p-3 light:border-gray-200 light:bg-white">
                <p class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500">Required scopes</p>
                <p class="mt-1 font-medium text-white light:text-gray-900">
                  {{
                    Array.isArray(preflightDetails.required_scopes)
                      ? preflightDetails.required_scopes.join(", ")
                      : "Not configured"
                  }}
                </p>
              </div>
            </div>
          </section>

          <section class="rounded-xl border border-white/[0.08] bg-white/[0.02] p-5 light:border-gray-200 light:bg-gray-50">
            <div class="flex items-start justify-between gap-4">
              <div>
                <h3 class="text-sm font-semibold text-white light:text-gray-900">2. Asset acquisition</h3>
                <p class="mt-1 text-sm text-white/60 light:text-gray-600">
                  Use Meta embedded signup by default, or fall back to manual asset import when you already have a token and IDs.
                </p>
              </div>
              <Badge variant="outline" :class="statusVariant(stepStatus('asset_acquisition'))">
                {{ stepStatus("asset_acquisition") }}
              </Badge>
            </div>

            <div class="mt-4 flex flex-wrap gap-2">
              <Button
                variant="outline"
                :class="mode === 'embedded_signup' ? 'border-emerald-500 text-emerald-500' : ''"
                @click="mode = 'embedded_signup'"
              >
                Connect via Meta
              </Button>
              <Button
                variant="outline"
                :class="mode === 'manual_import' ? 'border-emerald-500 text-emerald-500' : ''"
                @click="mode = 'manual_import'"
              >
                Manual import
              </Button>
            </div>

            <div v-if="mode === 'embedded_signup'" class="mt-5 space-y-4">
              <Alert variant="info">
                <AlertCircle class="h-4 w-4" />
                <AlertTitle>Preferred path</AlertTitle>
                <AlertDescription>
                  Launch Meta embedded signup first. If the popup finishes but Whatomate does not capture all values automatically, paste the returned code or access token and IDs below.
                </AlertDescription>
              </Alert>

              <Button :disabled="isWorking" @click="launchEmbeddedSignup">
                <Rocket class="mr-2 h-4 w-4" />
                Launch Meta Embedded Signup
              </Button>

              <div class="grid gap-4 md:grid-cols-2">
                <div class="space-y-2">
                  <Label>Account name</Label>
                  <Input v-model="embeddedSignupForm.account_name" placeholder="Support account" />
                </div>
                <div class="space-y-2">
                  <Label>Meta App ID</Label>
                  <Input v-model="embeddedSignupForm.app_id" placeholder="Meta App ID" />
                </div>
                <div class="space-y-2">
                  <Label>Authorization code</Label>
                  <Input v-model="embeddedSignupForm.code" placeholder="Paste Meta authorization code" />
                </div>
                <div class="space-y-2">
                  <Label>Business access token</Label>
                  <Input v-model="embeddedSignupForm.access_token" type="password" placeholder="Paste business access token if no code is available" />
                </div>
                <div class="space-y-2">
                  <Label>Phone number ID</Label>
                  <Input v-model="embeddedSignupForm.phone_id" placeholder="Phone number ID" />
                </div>
                <div class="space-y-2">
                  <Label>WABA ID</Label>
                  <Input v-model="embeddedSignupForm.business_id" placeholder="WhatsApp Business Account ID" />
                </div>
              </div>

              <Button :disabled="isWorking" @click="saveEmbeddedSignupResult()">
                <ClipboardCheck class="mr-2 h-4 w-4" />
                Save Embedded Signup Result
              </Button>
            </div>

            <div v-else class="mt-5 space-y-4">
              <div class="grid gap-4 md:grid-cols-2">
                <div class="space-y-2">
                  <Label>Account name</Label>
                  <Input v-model="manualImportForm.account_name" placeholder="Support account" />
                </div>
                <div class="space-y-2">
                  <Label>Meta App ID</Label>
                  <Input v-model="manualImportForm.app_id" placeholder="Meta App ID" />
                </div>
                <div class="space-y-2">
                  <Label>Meta App secret</Label>
                  <Input v-model="manualImportForm.app_secret" type="password" placeholder="Optional if app-level config already stores it" />
                </div>
                <div class="space-y-2">
                  <Label>Graph API version</Label>
                  <Input v-model="manualImportForm.api_version" placeholder="v21.0" />
                </div>
                <div class="space-y-2">
                  <Label>Phone number ID</Label>
                  <Input v-model="manualImportForm.phone_id" placeholder="Phone number ID" />
                </div>
                <div class="space-y-2">
                  <Label>WABA ID</Label>
                  <Input v-model="manualImportForm.business_id" placeholder="WhatsApp Business Account ID" />
                </div>
              </div>
              <div class="space-y-2">
                <Label>Business access token</Label>
                <Textarea v-model="manualImportForm.access_token" :rows="4" placeholder="Paste the business access token" />
              </div>
              <Button :disabled="isWorking" @click="importManual">
                <ClipboardCheck class="mr-2 h-4 w-4" />
                Import Meta Assets
              </Button>
            </div>
          </section>

          <section class="rounded-xl border border-white/[0.08] bg-white/[0.02] p-5 light:border-gray-200 light:bg-gray-50">
            <div class="flex items-start justify-between gap-4">
              <div>
                <h3 class="text-sm font-semibold text-white light:text-gray-900">3. Asset import</h3>
                <p class="mt-1 text-sm text-white/60 light:text-gray-600">
                  Whatomate creates or updates the tenant account record from the imported Meta values.
                </p>
              </div>
              <Badge variant="outline" :class="statusVariant(stepStatus('asset_import'))">
                {{ stepStatus("asset_import") }}
              </Badge>
            </div>

            <div class="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
              <div class="rounded-lg border border-white/[0.08] bg-black/20 p-3 light:border-gray-200 light:bg-white">
                <p class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500">Account</p>
                <p class="mt-1 font-medium text-white light:text-gray-900">{{ session.account_name || "Not imported" }}</p>
              </div>
              <div class="rounded-lg border border-white/[0.08] bg-black/20 p-3 light:border-gray-200 light:bg-white">
                <p class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500">Phone ID</p>
                <p class="mt-1 font-medium text-white light:text-gray-900">{{ session.phone_id || "Not imported" }}</p>
              </div>
              <div class="rounded-lg border border-white/[0.08] bg-black/20 p-3 light:border-gray-200 light:bg-white">
                <p class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500">WABA ID</p>
                <p class="mt-1 font-medium text-white light:text-gray-900">{{ session.business_id || "Not imported" }}</p>
              </div>
              <div class="rounded-lg border border-white/[0.08] bg-black/20 p-3 light:border-gray-200 light:bg-white">
                <p class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500">Verify token</p>
                <p class="mt-1 break-all font-medium text-white light:text-gray-900">{{ session.webhook_verify_token || "Generated after import" }}</p>
              </div>
            </div>
          </section>

          <section class="rounded-xl border border-white/[0.08] bg-white/[0.02] p-5 light:border-gray-200 light:bg-gray-50">
            <div class="flex items-start justify-between gap-4">
              <div>
                <h3 class="text-sm font-semibold text-white light:text-gray-900">4. Phone setup</h3>
                <p class="mt-1 text-sm text-white/60 light:text-gray-600">
                  Request and verify the OTP, then register the phone number with a 6-digit PIN.
                </p>
              </div>
              <Badge variant="outline" :class="statusVariant(stepStatus('phone_setup'))">
                {{ stepStatus("phone_setup") }}
              </Badge>
            </div>

            <p v-if="stepSummary('phone_setup')" class="mt-3 text-sm text-white/70 light:text-gray-700">
              {{ stepSummary("phone_setup") }}
            </p>

            <div class="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
              <div class="rounded-lg border border-white/[0.08] bg-black/20 p-3 light:border-gray-200 light:bg-white">
                <p class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500">Display number</p>
                <p class="mt-1 font-medium text-white light:text-gray-900">{{ phoneStepDetails.display_phone_number || "Unknown" }}</p>
              </div>
              <div class="rounded-lg border border-white/[0.08] bg-black/20 p-3 light:border-gray-200 light:bg-white">
                <p class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500">Verified name</p>
                <p class="mt-1 font-medium text-white light:text-gray-900">{{ phoneStepDetails.verified_name || "Unknown" }}</p>
              </div>
              <div class="rounded-lg border border-white/[0.08] bg-black/20 p-3 light:border-gray-200 light:bg-white">
                <p class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500">Verification</p>
                <p class="mt-1 font-medium text-white light:text-gray-900">{{ phoneStepDetails.code_verification_status || "Unknown" }}</p>
              </div>
              <div class="rounded-lg border border-white/[0.08] bg-black/20 p-3 light:border-gray-200 light:bg-white">
                <p class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500">Mode</p>
                <p class="mt-1 font-medium text-white light:text-gray-900">{{ phoneStepDetails.account_mode || "Unknown" }}</p>
              </div>
            </div>

            <div class="mt-5 grid gap-6 lg:grid-cols-3">
              <div class="space-y-3 rounded-lg border border-white/[0.08] bg-black/20 p-4 light:border-gray-200 light:bg-white">
                <div>
                  <p class="font-medium text-white light:text-gray-900">Request code</p>
                  <p class="text-sm text-white/60 light:text-gray-600">Ask Meta to send the OTP by SMS or voice.</p>
                </div>
                <div class="space-y-2">
                  <Label>Code method</Label>
                  <Input v-model="phoneSetupForm.code_method" placeholder="SMS or VOICE" />
                </div>
                <div class="space-y-2">
                  <Label>Language</Label>
                  <Input v-model="phoneSetupForm.language" placeholder="en_US" />
                </div>
                <Button :disabled="!hasImportedAccount || isWorking" @click="requestCode">Request code</Button>
              </div>

              <div class="space-y-3 rounded-lg border border-white/[0.08] bg-black/20 p-4 light:border-gray-200 light:bg-white">
                <div>
                  <p class="font-medium text-white light:text-gray-900">Verify code</p>
                  <p class="text-sm text-white/60 light:text-gray-600">Confirm the OTP you received from Meta.</p>
                </div>
                <div class="space-y-2">
                  <Label>Verification code</Label>
                  <Input v-model="phoneSetupForm.code" placeholder="123456" />
                </div>
                <Button :disabled="!hasImportedAccount || isWorking" @click="verifyCode">Verify code</Button>
              </div>

              <div class="space-y-3 rounded-lg border border-white/[0.08] bg-black/20 p-4 light:border-gray-200 light:bg-white">
                <div>
                  <p class="font-medium text-white light:text-gray-900">Register number</p>
                  <p class="text-sm text-white/60 light:text-gray-600">Enable messaging and set the initial two-step PIN.</p>
                </div>
                <div class="space-y-2">
                  <Label>6-digit PIN</Label>
                  <Input v-model="phoneSetupForm.pin" type="password" placeholder="123456" maxlength="6" />
                </div>
                <div class="space-y-2">
                  <Label>Backup password</Label>
                  <Input v-model="phoneSetupForm.backup_password" type="password" placeholder="Optional" />
                </div>
                <div class="space-y-2">
                  <Label>Backup data</Label>
                  <Textarea v-model="phoneSetupForm.backup_data" :rows="3" placeholder="Optional backup data" />
                </div>
                <Button :disabled="!hasImportedAccount || isWorking" @click="registerPhone">Register phone</Button>
              </div>
            </div>
          </section>

          <section class="rounded-xl border border-white/[0.08] bg-white/[0.02] p-5 light:border-gray-200 light:bg-gray-50">
            <div class="flex items-start justify-between gap-4">
              <div>
                <h3 class="text-sm font-semibold text-white light:text-gray-900">5. Webhooks</h3>
                <p class="mt-1 text-sm text-white/60 light:text-gray-600">
                  Validate the public callback URL, then subscribe the app to Meta webhooks.
                </p>
              </div>
              <Badge variant="outline" :class="statusVariant(stepStatus('webhooks'))">
                {{ stepStatus("webhooks") }}
              </Badge>
            </div>

            <div class="mt-4 grid gap-3 md:grid-cols-2">
              <div class="rounded-lg border border-white/[0.08] bg-black/20 p-3 light:border-gray-200 light:bg-white">
                <p class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500">Callback URL</p>
                <p class="mt-1 break-all font-medium text-white light:text-gray-900">
                  {{ webhookStepDetails.callback_url || preflightDetails.callback_url || "Not configured" }}
                </p>
              </div>
              <div class="rounded-lg border border-white/[0.08] bg-black/20 p-3 light:border-gray-200 light:bg-white">
                <p class="text-xs uppercase tracking-wide text-white/40 light:text-gray-500">Verify token</p>
                <p class="mt-1 break-all font-medium text-white light:text-gray-900">
                  {{ session.webhook_verify_token || "Generated after import" }}
                </p>
              </div>
            </div>

            <div class="mt-5 flex flex-wrap gap-3">
              <Button :disabled="!hasImportedAccount || isWorking" @click="validateWebhook">
                <Link2 class="mr-2 h-4 w-4" />
                Validate callback
              </Button>
              <Button :disabled="!hasImportedAccount || isWorking" variant="outline" @click="subscribeWebhooks">
                <ShieldCheck class="mr-2 h-4 w-4" />
                Subscribe app
              </Button>
            </div>
          </section>

          <section class="rounded-xl border border-white/[0.08] bg-white/[0.02] p-5 light:border-gray-200 light:bg-gray-50">
            <div class="flex items-start justify-between gap-4">
              <div>
                <h3 class="text-sm font-semibold text-white light:text-gray-900">6. Connection test</h3>
                <p class="mt-1 text-sm text-white/60 light:text-gray-600">
                  Run the final readiness check. Whatomate validates the account, refreshes phone status, and computes the final onboarding state.
                </p>
              </div>
              <Badge variant="outline" :class="statusVariant(stepStatus('connection_test'))">
                {{ stepStatus("connection_test") }}
              </Badge>
            </div>

            <div class="mt-5">
              <Button :disabled="!hasImportedAccount || isWorking" @click="finalizeOnboarding">
                <TestTube2 class="mr-2 h-4 w-4" />
                Run final checks
              </Button>
            </div>
          </section>

          <section class="rounded-xl border border-white/[0.08] bg-white/[0.02] p-5 light:border-gray-200 light:bg-gray-50">
            <div class="flex items-start justify-between gap-4">
              <div>
                <h3 class="text-sm font-semibold text-white light:text-gray-900">7. Final status</h3>
                <p class="mt-1 text-sm text-white/60 light:text-gray-600">
                  Ready means the internal setup is complete. Waiting on Meta means the technical setup is done but a Meta-controlled checkpoint is still pending.
                </p>
              </div>
              <Badge variant="outline" :class="statusVariant(session.readiness?.status || stepStatus('final_status'))">
                {{ session.readiness?.status || stepStatus("final_status") }}
              </Badge>
            </div>

            <p v-if="session.readiness?.summary" class="mt-3 text-sm text-white/70 light:text-gray-700">
              {{ session.readiness.summary }}
            </p>

            <div v-if="session.readiness?.external_checkpoints?.length" class="mt-4 space-y-3">
              <div
                v-for="checkpoint in session.readiness.external_checkpoints"
                :key="checkpoint.key"
                class="rounded-lg border border-white/[0.08] bg-black/20 p-3 light:border-gray-200 light:bg-white"
              >
                <div class="flex items-center justify-between gap-3">
                  <p class="font-medium text-white light:text-gray-900">{{ checkpoint.label }}</p>
                  <Badge variant="outline" :class="statusVariant(checkpoint.status)">
                    {{ checkpoint.status }}
                  </Badge>
                </div>
                <p v-if="checkpoint.summary" class="mt-2 text-sm text-white/60 light:text-gray-600">
                  {{ checkpoint.summary }}
                </p>
              </div>
            </div>
          </section>
        </template>
      </div>
    </DialogScrollContent>
  </Dialog>
</template>
