<script setup lang="ts">
import { isAxiosError } from 'axios';
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useForgotPasswordStore } from '../stores/forgotPassword';

const router = useRouter();
const forgotPassword = useForgotPasswordStore();

const email = ref(forgotPassword.email);
const errorMessage = ref('');
const isSubmitting = ref(false);

onMounted(() => {
  const savedTheme = localStorage.getItem('fastfiles-theme');
  document.documentElement.dataset.theme = savedTheme === 'dark' ? 'dark' : 'light';
});

async function handleSubmit() {
  const normalizedEmail = email.value.trim();

  if (!normalizedEmail || isSubmitting.value) return;

  isSubmitting.value = true;
  errorMessage.value = '';

  try {
    await forgotPassword.startForgotPassword(normalizedEmail);
    await router.push('/user/forgot-password/verify-otp');
  } catch (error: unknown) {
    if (isAxiosError<{ message?: string }>(error)) {
      errorMessage.value =
        error.response?.data?.message ?? 'Could not send the code. Please try again.';
    } else {
      errorMessage.value = 'Something went wrong. Please try again.';
    }
  } finally {
    isSubmitting.value = false;
  }
}
</script>

<template>
  <main class="recovery-page">
    <section class="recovery-card">
      <a class="brand" href="/" aria-label="FastFiles home">
        <span class="brand-mark">F</span>
        <span>FastFiles</span>
      </a>

      <p class="eyebrow">ACCOUNT RECOVERY</p>
      <h1>Forgot your password?</h1>
      <p class="intro">
        Enter the email address on your account and we’ll send you a verification code.
      </p>

      <form class="recovery-form" @submit.prevent="handleSubmit">
        <label for="email">Email address</label>
        <input
          id="email"
          v-model="email"
          type="email"
          autocomplete="email"
          placeholder="you@example.com"
          required
        />

        <p v-if="errorMessage" class="error-message" role="alert">
          {{ errorMessage }}
        </p>

        <button class="submit-button" type="submit" :disabled="!email.trim() || isSubmitting">
          {{ isSubmitting ? 'Sending code…' : 'Send verification code' }}
        </button>
      </form>

      <RouterLink class="back-link" to="/user/login">
        Back to sign in
      </RouterLink>
    </section>
  </main>
</template>

<style scoped>
.recovery-page {
  display: grid;
  min-height: 100vh;
  place-items: center;
  padding: 28px 18px;
  background:
    radial-gradient(ellipse at 15% 10%, var(--color-primary-soft), transparent 45%),
    var(--color-bg);
}

.recovery-card {
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

.intro {
  margin: 10px 0 26px;
  color: var(--color-text-muted);
  font-size: 14px;
  line-height: 1.6;
}

.recovery-form {
  display: grid;
  gap: 10px;
}

.recovery-form label {
  margin-top: 4px;
  color: var(--color-text);
  font-size: 13px;
  font-weight: 650;
}

.recovery-form input {
  width: 100%;
  min-height: 46px;
  padding: 0 13px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
  background: var(--color-bg);
  color: var(--color-text);
}

.error-message {
  margin: 4px 0 0;
  color: var(--color-danger);
  font-size: 13px;
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

.back-link {
  display: block;
  margin-top: 22px;
  color: var(--color-primary);
  font-size: 13px;
  font-weight: 650;
  text-align: center;
}

@media (max-width: 480px) {
  .recovery-card {
    padding: 30px 24px;
  }
}
</style>