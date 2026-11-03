<script setup lang="ts">
import { isAxiosError } from 'axios';
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue';
import type { ComponentPublicInstance } from 'vue';
import { useRouter } from 'vue-router';
import { appConfig } from '../config/env';
import { useForgotPasswordStore } from '../stores/forgotPassword';

const otpLength = 6;
const expiredStatuses = [410, 422, 429];

const router = useRouter();
const forgot = useForgotPasswordStore();
const digits = ref(Array.from({ length: otpLength }, () => ''));
const inputRefs = ref<HTMLInputElement[]>([]);
const cooldown = ref(0);
const isSubmitting = ref(false);
const isResending = ref(false);
const errorMessage = ref('');
const flowExpired = ref(false);
const redirectSeconds = ref(5);

let cooldownInterval: number | undefined;
let redirectInterval: number | undefined;

const fullOtp = computed(() => digits.value.join(''));

function setInputRef(
  element: Element | ComponentPublicInstance | null,
  index: number,
) {
  if (element instanceof HTMLInputElement) {
    inputRefs.value[index] = element;
  }
}

function startCooldown() {
  window.clearInterval(cooldownInterval);

  cooldown.value = appConfig.resendOtpCooldownSeconds;
  cooldownInterval = window.setInterval(() => {
    if (cooldown.value <= 1) {
      cooldown.value = 0;
      window.clearInterval(cooldownInterval);
    } else {
      cooldown.value -= 1;
    }
  }, 1000);
}

function startExpiredRedirect(message: string) {
  if (flowExpired.value) return;

  flowExpired.value = true;
  errorMessage.value = message;

  redirectInterval = window.setInterval(() => {
    redirectSeconds.value -= 1;

    if (redirectSeconds.value <= 0) {
      window.clearInterval(redirectInterval);
      forgot.resetFlow();
      void router.push('/user/login');
    }
  }, 1000);
}

function handleFlowError(error: unknown, fallback: string) {
  if (isAxiosError<{ message?: string }>(error)) {
    const status = error.response?.status;

    if (status && expiredStatuses.includes(status)) {
      startExpiredRedirect(
        error.response?.data?.message ?? 'This verification flow has expired.',
      );
      return;
    }

    errorMessage.value = error.response?.data?.message ?? fallback;
    return;
  }

  errorMessage.value = fallback;
}

function handleInput(index: number, event: Event) {
  const target = event.target as HTMLInputElement;
  const entered = target.value.replace(/\D/g, '');

  if (entered.length > 1) {
    entered.split('').slice(0, otpLength - index).forEach((digit, offset) => {
      digits.value[index + offset] = digit;
    });
  } else {
    digits.value[index] = entered;
  }

  target.value = digits.value[index] ?? '';

  const nextIndex = Math.min(index + Math.max(entered.length, 1), otpLength - 1);
  void nextTick(() => inputRefs.value[nextIndex]?.focus());
}

function handlePaste(index: number, event: ClipboardEvent) {
  event.preventDefault();

  const pasted = event.clipboardData?.getData('text').replace(/\D/g, '') ?? '';
  pasted.split('').slice(0, otpLength - index).forEach((digit, offset) => {
    digits.value[index + offset] = digit;
  });

  const nextIndex = Math.min(index + pasted.length, otpLength - 1);
  void nextTick(() => inputRefs.value[nextIndex]?.focus());
}

function handleKeydown(index: number, event: KeyboardEvent) {
  if (event.key === 'Backspace' && !digits.value[index] && index > 0) {
    inputRefs.value[index - 1]?.focus();
  } else if (event.key === 'ArrowLeft' && index > 0) {
    inputRefs.value[index - 1]?.focus();
  } else if (event.key === 'ArrowRight' && index < otpLength - 1) {
    inputRefs.value[index + 1]?.focus();
  }
}

async function verifyOtp() {
  if (fullOtp.value.length !== otpLength || isSubmitting.value) return;

  isSubmitting.value = true;
  errorMessage.value = '';

  try {
    await forgot.verifyOtp(fullOtp.value);
    await router.push('/user/forgot-password/reset-password');
  } catch (error: unknown) {
    handleFlowError(error, 'That code could not be verified. Please try again.');
    digits.value = Array.from({ length: otpLength }, () => '');
    void nextTick(() => inputRefs.value[0]?.focus());
  } finally {
    isSubmitting.value = false;
  }
}

async function resendOtp() {
  if (cooldown.value > 0 || isResending.value) return;

  isResending.value = true;
  errorMessage.value = '';

  try {
    await forgot.resendOtp();
    startCooldown();
  } catch (error: unknown) {
    handleFlowError(error, 'Could not resend the code. Please try again.');
  } finally {
    isResending.value = false;
  }
}

onMounted(() => {
  const savedTheme = localStorage.getItem('fastfiles-theme');
  document.documentElement.dataset.theme = savedTheme === 'dark' ? 'dark' : 'light';

  if (!forgot.flowId || !forgot.email) {
    void router.replace('/user/forgot-password');
    return;
  }

  startCooldown();
  void nextTick(() => inputRefs.value[0]?.focus());
});

onUnmounted(() => {
  window.clearInterval(cooldownInterval);
  window.clearInterval(redirectInterval);
});
</script>

<template>
  <main class="otp-page">
    <section v-if="flowExpired" class="otp-card expired-card">
      <p class="eyebrow">ACCOUNT RECOVERY</p>
      <h1>Verification expired</h1>
      <p>{{ errorMessage }}</p>
      <p>Returning to sign in in {{ redirectSeconds }} seconds…</p>
    </section>

    <section v-else class="otp-card">
      <a class="brand" href="/" aria-label="FastFiles home">
        <span class="brand-mark">F</span>
        <span>FastFiles</span>
      </a>

      <p class="eyebrow">ACCOUNT RECOVERY</p>
      <h1>Check your email</h1>
      <p class="intro">
        Enter the six-digit code sent to <strong>{{ forgot.email }}</strong>.
      </p>

      <form @submit.prevent="verifyOtp">
        <div class="otp-inputs" aria-label="Six-digit verification code">
          <input
            v-for="(_, index) in digits"
            :key="index"
            :ref="(element) => setInputRef(element, index)"
            :value="digits[index]"
            type="text"
            inputmode="numeric"
            autocomplete="one-time-code"
            maxlength="1"
            :aria-label="`Code digit ${index + 1}`"
            @input="handleInput(index, $event)"
            @paste="handlePaste(index, $event)"
            @keydown="handleKeydown(index, $event)"
          />
        </div>

        <p v-if="errorMessage" class="error-message" role="alert">
          {{ errorMessage }}
        </p>

        <button
          class="submit-button"
          type="submit"
          :disabled="fullOtp.length !== otpLength || isSubmitting"
        >
          {{ isSubmitting ? 'Verifying…' : 'Verify code' }}
        </button>
      </form>

      <button
        class="resend-button"
        type="button"
        :disabled="cooldown > 0 || isResending"
        @click="resendOtp"
      >
        {{
          isResending
            ? 'Sending…'
            : cooldown > 0
              ? `Resend code in ${cooldown}s`
              : 'Resend code'
        }}
      </button>

      <RouterLink class="back-link" to="/user/login" @click="forgot.resetFlow()">
        Back to sign in
      </RouterLink>
    </section>
  </main>
</template>

<style scoped>
.otp-page {
  display: grid;
  min-height: 100vh;
  place-items: center;
  padding: 28px 18px;
  background:
    radial-gradient(ellipse at 15% 10%, var(--color-primary-soft), transparent 45%),
    var(--color-bg);
}

.otp-card {
  width: min(100%, 440px);
  padding: 40px;
  border: 1px solid var(--color-border);
  border-radius: 22px;
  background: var(--color-surface);
  box-shadow: var(--shadow-card);
}

.brand {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 42px;
  color: var(--color-text);
  font-size: 18px;
  font-weight: 750;
  text-decoration: none;
}

.brand-mark {
  display: grid;
  width: 34px;
  height: 34px;
  place-items: center;
  border-radius: 11px;
  background: var(--color-primary);
  color: var(--color-on-primary);
  font-weight: 800;
}

.eyebrow {
  margin: 0 0 8px;
  color: var(--color-primary);
  font-size: 10px;
  font-weight: 750;
  letter-spacing: 1.2px;
}

h1 {
  margin: 0;
  color: var(--color-text);
  font-size: 28px;
  letter-spacing: -0.7px;
}

.intro,
.expired-card > p {
  color: var(--color-text-muted);
  font-size: 14px;
  line-height: 1.6;
}

.otp-inputs {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  margin: 26px 0 18px;
}

.otp-inputs input {
  width: 48px;
  height: 54px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
  background: var(--color-bg);
  color: var(--color-text);
  font-size: 22px;
  text-align: center;
}

.otp-inputs input:focus {
  border-color: var(--color-focus);
}

.error-message {
  color: var(--color-danger);
  font-size: 13px;
}

.submit-button {
  width: 100%;
  min-height: 46px;
  border: 0;
  border-radius: var(--radius-control);
  background: var(--color-primary);
  color: var(--color-on-primary);
  font-weight: 700;
  cursor: pointer;
}

.submit-button:disabled,
.resend-button:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.resend-button {
  display: block;
  min-height: 40px;
  padding: 0;
  margin: 12px auto 0;
  border: 0;
  background: transparent;
  color: var(--color-primary);
  font-weight: 650;
  cursor: pointer;
}

.back-link {
  display: block;
  margin-top: 16px;
  color: var(--color-text-muted);
  font-size: 13px;
  text-align: center;
}

@media (max-width: 480px) {
  .otp-card {
    padding: 30px 22px;
  }

  .otp-inputs input {
    width: 42px;
  }
}
</style>