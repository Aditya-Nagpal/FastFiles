<script setup lang="ts">
import { isAxiosError } from 'axios';
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useForgotPasswordStore } from '../stores/forgotPassword';

const router = useRouter();
const forgot = useForgotPasswordStore();

const password = ref('');
const confirmPassword = ref('');
const errorMessage = ref('');
const isSubmitting = ref(false);
const flowExpired = ref(false);
const redirectSeconds = ref(5);

let redirectInterval: number | undefined;

const passwordPattern =
  /^(?=.*[a-z])(?=.*[A-Z])(?=.*\d)(?=.*[@$!%*?&])[A-Za-z\d@$!%*?&]{8,}$/;

const passwordIsStrong = computed(() => passwordPattern.test(password.value));

const canSubmit = computed(
  () =>
    passwordIsStrong.value &&
    password.value === confirmPassword.value &&
    !isSubmitting.value,
);

onMounted(() => {
  const savedTheme = localStorage.getItem('fastfiles-theme');
  document.documentElement.dataset.theme = savedTheme === 'dark' ? 'dark' : 'light';

  if (!forgot.flowId || !forgot.email) {
    void router.replace('/user/forgot-password');
  }
});

onUnmounted(() => {
  window.clearInterval(redirectInterval);
});

function startExpiredRedirect(message: string) {
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

async function handleSubmit() {
  if (!canSubmit.value) return;

  isSubmitting.value = true;
  errorMessage.value = '';

  try {
    await forgot.resetPassword(password.value);
    await router.push('/user/login');
  } catch (error: unknown) {
    if (isAxiosError<{ message?: string }>(error)) {
      const status = error.response?.status;

      if (status && [410, 422, 429].includes(status)) {
        startExpiredRedirect(
          error.response?.data?.message ?? 'This password reset flow has expired.',
        );
      } else {
        errorMessage.value =
          error.response?.data?.message ?? 'Could not update your password. Please try again.';
      }
    } else {
      errorMessage.value = 'Something went wrong. Please try again.';
    }
  } finally {
    isSubmitting.value = false;
  }
}
</script>

<template>
  <main class="reset-page">
    <section v-if="flowExpired" class="reset-card">
      <p class="eyebrow">ACCOUNT RECOVERY</p>
      <h1>Reset flow expired</h1>
      <p class="intro">{{ errorMessage }}</p>
      <p class="redirect-note">Returning to sign in in {{ redirectSeconds }} seconds…</p>
    </section>

    <section v-else class="reset-card">
      <a class="brand" href="/" aria-label="FastFiles home">
        <span class="brand-mark">F</span>
        <span>FastFiles</span>
      </a>

      <p class="eyebrow">ACCOUNT RECOVERY</p>
      <h1>Choose a new password</h1>
      <p class="intro">
        Set a strong password for <strong>{{ forgot.email }}</strong>.
      </p>

      <form class="reset-form" @submit.prevent="handleSubmit">
        <label for="password">New password</label>
        <input
          id="password"
          v-model="password"
          type="password"
          autocomplete="new-password"
          required
        />
        <span class="hint">
          At least 8 characters with uppercase, lowercase, number, and one of @$!%*?&.
        </span>

        <label for="confirm-password">Confirm new password</label>
        <input
          id="confirm-password"
          v-model="confirmPassword"
          type="password"
          autocomplete="new-password"
          required
        />

        <p v-if="password && !passwordIsStrong" class="field-error">
          Your password does not meet the requirements above.
        </p>
        <p
          v-if="confirmPassword && password !== confirmPassword"
          class="field-error"
        >
          Passwords do not match.
        </p>

        <p v-if="errorMessage" class="error-message" role="alert">
          {{ errorMessage }}
        </p>

        <button class="submit-button" type="submit" :disabled="!canSubmit">
          {{ isSubmitting ? 'Updating password…' : 'Update password' }}
        </button>
      </form>
    </section>
  </main>
</template>

<style scoped>
.reset-page {
  display: grid;
  min-height: 100vh;
  place-items: center;
  padding: 28px 18px;
  background:
    radial-gradient(ellipse at 15% 10%, var(--color-primary-soft), transparent 45%),
    var(--color-bg);
}

.reset-card {
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
  font-size: 27px;
  letter-spacing: -0.7px;
}

.intro,
.redirect-note {
  margin: 10px 0 25px;
  color: var(--color-text-muted);
  font-size: 14px;
  line-height: 1.6;
}

.reset-form {
  display: grid;
  gap: 10px;
}

.reset-form label {
  margin-top: 6px;
  color: var(--color-text);
  font-size: 13px;
  font-weight: 650;
}

.reset-form input {
  width: 100%;
  min-height: 46px;
  padding: 0 13px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
  background: var(--color-bg);
  color: var(--color-text);
}

.hint,
.field-error,
.error-message {
  font-size: 12px;
  line-height: 1.5;
}

.hint {
  margin: -4px 0 5px;
  color: var(--color-text-subtle);
}

.field-error,
.error-message {
  margin: 2px 0;
  color: var(--color-danger);
}

.submit-button {
  min-height: 46px;
  margin-top: 10px;
  border: 0;
  border-radius: var(--radius-control);
  background: var(--color-primary);
  color: var(--color-on-primary);
  font-size: 14px;
  font-weight: 700;
  cursor: pointer;
}

.submit-button:hover:not(:disabled) {
  background: var(--color-primary-hover);
}

.submit-button:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

@media (max-width: 480px) {
  .reset-card {
    padding: 30px 24px;
  }
}
</style>