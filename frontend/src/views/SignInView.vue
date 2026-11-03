<script setup lang="ts">
import { isAxiosError } from 'axios';
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useAuthStore } from '../stores/auth';
import { useUserStore } from '../stores/user';

const router = useRouter();
const auth = useAuthStore();
const user = useUserStore();

const email = ref('');
const password = ref('');
const isSubmitting = ref(false);
const errorMessage = ref('');

const canSubmit = computed(
  () => email.value.trim().length > 0 && password.value.length > 0,
);

onMounted(() => {
  const savedTheme = localStorage.getItem('fastfiles-theme');

  if (savedTheme === 'dark') {
    document.documentElement.dataset.theme = 'dark';
  } else {
    document.documentElement.dataset.theme = 'light';
  }
});

async function handleSubmit() {
  if (!canSubmit.value || isSubmitting.value) return;

  isSubmitting.value = true;
  errorMessage.value = '';

  try {
    await auth.signIn(email.value.trim(), password.value);

    try {
      await user.fetchUserProfile();
    } catch (profileError) {
      console.error('Could not load the user profile:', profileError);
    }

    const redirect = router.currentRoute.value.query.redirect;
    await router.replace(typeof redirect === 'string' ? redirect : '/');
  } catch (error: unknown) {
    if (isAxiosError<{ message?: string }>(error)) {
      errorMessage.value =
        error.response?.data?.message ?? 'Sign-in failed. Check your details and try again.';
    } else {
      errorMessage.value = 'Something went wrong. Please try again.';
    }
  } finally {
    isSubmitting.value = false;
  }
}
</script>

<template>
  <main class="signin-page">
    <section class="signin-card">
      <a class="brand" href="/" aria-label="FastFiles home">
        <span class="brand-mark">F</span>
        <span>FastFiles</span>
      </a>

      <p class="eyebrow">WELCOME BACK</p>
      <h1>Sign in to your files</h1>
      <p class="intro">Pick up right where you left off.</p>

      <form class="signin-form" @submit.prevent="handleSubmit">
        <label for="email">Email address</label>
        <input id="email" v-model="email" type="email" autocomplete="email" placeholder="you@example.com" required />

        <label for="password">Password</label>
        <input id="password" v-model="password" type="password" autocomplete="current-password"
          placeholder="Enter your password" required />

        <p v-if="errorMessage" class="error-message" role="alert">
          {{ errorMessage }}
        </p>



        <button class="submit-button" type="submit" :disabled="!canSubmit || isSubmitting">
          {{ isSubmitting ? 'Signing in…' : 'Sign in' }}
        </button>
      </form>

      <RouterLink class="forgot-link" to="/user/forgot-password">
        Forgot password?
      </RouterLink>

      <p class="next-step-note">
        Registration and password recovery screens are coming next.
      </p>
    </section>
  </main>
</template>

<style scoped>
.signin-page {
  display: grid;
  min-height: 100vh;
  place-items: center;
  padding: 28px 18px;
  background:
    radial-gradient(ellipse at 15% 10%, var(--color-primary-soft), transparent 45%),
    var(--color-bg);
}

.signin-card {
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
  font-size: 29px;
  letter-spacing: -0.8px;
}

.intro {
  margin: 9px 0 28px;
  color: var(--color-text-muted);
  font-size: 14px;
}

.signin-form {
  display: grid;
  gap: 10px;
}

.signin-form label {
  margin-top: 8px;
  color: var(--color-text);
  font-size: 13px;
  font-weight: 650;
}

.signin-form input {
  width: 100%;
  min-height: 46px;
  padding: 0 13px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
  background: var(--color-bg);
  color: var(--color-text);
}

.signin-form input::placeholder {
  color: var(--color-text-subtle);
}

.signin-form input:focus {
  border-color: var(--color-focus);
  outline: 3px solid color-mix(in srgb, var(--color-focus) 22%, transparent);
}

.error-message {
  margin: 4px 0 0;
  color: var(--color-danger);
  font-size: 13px;
}

.submit-button {
  min-height: 46px;
  margin-top: 12px;
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

.next-step-note {
  margin: 22px 0 0;
  color: var(--color-text-subtle);
  font-size: 12px;
  line-height: 1.5;
  text-align: center;
}

@media (max-width: 480px) {
  .signin-card {
    padding: 30px 24px;
  }

  .brand {
    margin-bottom: 34px;
  }
}
</style>