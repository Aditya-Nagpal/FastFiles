<script setup lang="ts">
import { isAxiosError } from 'axios';
import { computed, onMounted, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useAuthStore } from '../stores/auth';

interface Country {
  name: string;
  code: string;
}

interface CountryApiItem {
  name?: { common?: string };
  idd?: { root?: string; suffixes?: string[] };
}

const auth = useAuthStore();
const router = useRouter();

const countries = ref<Country[]>([{ name: 'India', code: '+91' }]);
const errorMessage = ref('');
const isSubmitting = ref(false);
const maxAboutLength = 500;

const form = reactive({
  firstName: '',
  lastName: '',
  email: '',
  alternateEmail: '',
  contactNumber: '',
  country: 'India',
  gender: '',
  dob: '',
  timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
  about: '',
  plan: '',
  password: '',
  confirmPassword: '',
  termsAndPrivacy: false,
});

const genderOptions = [
  { value: 'male', label: 'Male' },
  { value: 'female', label: 'Female' },
  { value: 'other', label: 'Other' },
  { value: 'prefer_not_to_say', label: 'Prefer not to say' },
];

const planOptions = ['free', 'pro', 'business'];

const selectedCountry = computed(
  () => countries.value.find((country) => country.name === form.country) ?? countries.value[0],
);

const passwordIsStrong = computed(() =>
  /^(?=.*[a-z])(?=.*[A-Z])(?=.*\d)(?=.*[@$!%*?&])[A-Za-z\d@$!%*?&]{8,}$/.test(
    form.password,
  ),
);

const canSubmit = computed(
  () =>
    form.firstName.trim() &&
    form.lastName.trim() &&
    form.email.trim() &&
    form.contactNumber.length === 10 &&
    form.gender &&
    form.dob &&
    form.timezone &&
    form.plan &&
    passwordIsStrong.value &&
    form.password === form.confirmPassword &&
    form.termsAndPrivacy,
);

onMounted(async () => {
  const savedTheme = localStorage.getItem('fastfiles-theme');
  document.documentElement.dataset.theme = savedTheme === 'dark' ? 'dark' : 'light';

  try {
    const response = await fetch(
      'https://restcountries.com/v3.1/all?fields=name,idd',
    );

    if (!response.ok) return;

    const data = (await response.json()) as CountryApiItem[];

    const options = data
      .map((item) => {
        const root = item.idd?.root;
        const suffix = item.idd?.suffixes?.[0];
        const name = item.name?.common;

        if (!root || !name) return null;

        return {
          name,
          code: `${root}${suffix ?? ''}`,
        };
      })
      .filter((country): country is Country => country !== null)
      .sort((a, b) => a.name.localeCompare(b.name));

    if (options.length > 0) {
      countries.value = options;
    }
  } catch {
    // Keep India as the fallback if the country list cannot be loaded.
  }
});

function formatName(value: string): string {
  const trimmed = value.trim();

  return trimmed
    ? trimmed.charAt(0).toUpperCase() + trimmed.slice(1).toLowerCase()
    : '';
}

function updatePhone(value: string) {
  form.contactNumber = value.replace(/\D/g, '').slice(0, 10);
}

async function handleSubmit() {
  if (!canSubmit.value || isSubmitting.value) return;

  if (form.alternateEmail && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.alternateEmail)) {
    errorMessage.value = 'Enter a valid alternate email address.';
    return;
  }

  isSubmitting.value = true;
  errorMessage.value = '';

  const payload = {
    name: `${formatName(form.firstName)} ${formatName(form.lastName)}`,
    email: form.email.trim(),
    alternateEmail: form.alternateEmail.trim(),
    contactNumber: `${selectedCountry.value?.code}${form.contactNumber}`,
    country: selectedCountry.value?.name,
    gender: form.gender,
    dob: form.dob,
    timezone: form.timezone,
    about: form.about,
    plan: form.plan,
    password: form.password,
    termsAndPrivacy: form.termsAndPrivacy,
  };

  try {
    await auth.signUp(payload);
    await router.push('/user/login');
  } catch (error: unknown) {
    if (isAxiosError<{ message?: string }>(error)) {
      errorMessage.value =
        error.response?.data?.message ?? 'Sign-up failed. Please try again.';
    } else {
      errorMessage.value = 'Something went wrong. Please try again.';
    }
  } finally {
    isSubmitting.value = false;
  }
}
</script>

<template>
  <main class="signup-page">
    <section class="signup-card">
      <a class="brand" href="/" aria-label="FastFiles home">
        <span class="brand-mark">F</span>
        <span>FastFiles</span>
      </a>

      <p class="eyebrow">GET STARTED</p>
      <h1>Create your account</h1>
      <p class="intro">A few details, then your files have a new home.</p>

      <form class="signup-form" @submit.prevent="handleSubmit">
        <div class="form-grid">
          <label>
            First name
            <input v-model="form.firstName" autocomplete="given-name" required />
          </label>

          <label>
            Last name
            <input v-model="form.lastName" autocomplete="family-name" required />
          </label>

          <label>
            Email address
            <input
              v-model="form.email"
              type="email"
              autocomplete="email"
              required
            />
          </label>

          <label>
            Alternate email <span class="optional">(optional)</span>
            <input
              v-model="form.alternateEmail"
              type="email"
              autocomplete="email"
            />
          </label>

          <label>
            Country
            <select v-model="form.country">
              <option v-for="country in countries" :key="country.name" :value="country.name">
                {{ country.name }} ({{ country.code }})
              </option>
            </select>
          </label>

          <label>
            Phone number
            <div class="phone-field">
              <span>{{ selectedCountry?.code }}</span>
              <input
                :value="form.contactNumber"
                type="tel"
                inputmode="numeric"
                autocomplete="tel-national"
                maxlength="10"
                placeholder="10 digit number"
                required
                @input="updatePhone(($event.target as HTMLInputElement).value)"
              />
            </div>
          </label>

          <label>
            Gender
            <select v-model="form.gender" required>
              <option disabled value="">Select gender</option>
              <option v-for="option in genderOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>

          <label>
            Date of birth
            <input v-model="form.dob" type="date" required />
          </label>

          <label>
            Timezone
            <input v-model="form.timezone" required />
          </label>

          <label>
            Plan
            <select v-model="form.plan" required>
              <option disabled value="">Select plan</option>
              <option v-for="plan in planOptions" :key="plan" :value="plan">
                {{ plan.charAt(0).toUpperCase() + plan.slice(1) }}
              </option>
            </select>
          </label>

          <label class="full-width">
            About <span class="optional">(optional)</span>
            <textarea
              v-model="form.about"
              rows="3"
              :maxlength="maxAboutLength"
              placeholder="A little about yourself"
            />
            <span class="character-count">{{ form.about.length }} / {{ maxAboutLength }}</span>
          </label>

          <label>
            Password
            <input
              v-model="form.password"
              type="password"
              autocomplete="new-password"
              required
            />
            <span class="field-hint">8+ characters, uppercase, lowercase, number, and @$!%*?&</span>
          </label>

          <label>
            Confirm password
            <input
              v-model="form.confirmPassword"
              type="password"
              autocomplete="new-password"
              required
            />
            <span v-if="form.confirmPassword && form.password !== form.confirmPassword" class="field-error">
              Passwords do not match.
            </span>
          </label>
        </div>

        <label class="terms">
          <input v-model="form.termsAndPrivacy" type="checkbox" required />
          <span>I agree to the Terms and Privacy Policy.</span>
        </label>

        <p v-if="errorMessage" class="error-message" role="alert">
          {{ errorMessage }}
        </p>

        <button class="submit-button" type="submit" :disabled="!canSubmit || isSubmitting">
          {{ isSubmitting ? 'Creating account…' : 'Create account' }}
        </button>
      </form>

      <p class="signin-link">
        Already have an account?
        <RouterLink to="/user/login">Sign in</RouterLink>
      </p>
    </section>
  </main>
</template>

<style scoped>
.signup-page {
  display: grid;
  min-height: 100vh;
  place-items: center;
  padding: 32px 18px;
  background:
    radial-gradient(ellipse at 12% 8%, var(--color-primary-soft), transparent 42%),
    var(--color-bg);
}

.signup-card {
  width: min(100%, 820px);
  padding: 38px;
  border: 1px solid var(--color-border);
  border-radius: 22px;
  background: var(--color-surface);
  box-shadow: var(--shadow-card);
}

.brand {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 30px;
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
  margin: 9px 0 26px;
  color: var(--color-text-muted);
  font-size: 14px;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 18px;
}

.form-grid label {
  display: grid;
  align-content: start;
  gap: 8px;
  color: var(--color-text);
  font-size: 13px;
  font-weight: 650;
}

.form-grid input,
.form-grid select,
.form-grid textarea {
  width: 100%;
  min-height: 44px;
  padding: 10px 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
  background: var(--color-bg);
  color: var(--color-text);
}

.form-grid textarea {
  resize: vertical;
}

.form-grid input:focus,
.form-grid select:focus,
.form-grid textarea:focus {
  border-color: var(--color-focus);
  outline: 3px solid color-mix(in srgb, var(--color-focus) 22%, transparent);
}

.full-width {
  grid-column: 1 / -1;
}

.optional,
.field-hint,
.character-count {
  color: var(--color-text-subtle);
  font-size: 11px;
  font-weight: 400;
}

.character-count {
  justify-self: end;
}

.phone-field {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 44px;
  padding-left: 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
  background: var(--color-bg);
  color: var(--color-text-muted);
}

.phone-field input {
  min-width: 0;
  border: 0;
  background: transparent;
}

.phone-field input:focus {
  outline: none;
}

.field-error,
.error-message {
  color: var(--color-danger);
  font-size: 12px;
  font-weight: 400;
}

.terms {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin: 24px 0 18px;
  color: var(--color-text-muted);
  font-size: 13px;
}

.terms input {
  accent-color: var(--color-primary);
}

.submit-button {
  width: 100%;
  min-height: 46px;
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

.signin-link {
  margin: 20px 0 0;
  color: var(--color-text-muted);
  font-size: 13px;
  text-align: center;
}

.signin-link a {
  margin-left: 4px;
  color: var(--color-primary);
  font-weight: 700;
}

@media (max-width: 620px) {
  .signup-card {
    padding: 28px 22px;
  }

  .form-grid {
    grid-template-columns: 1fr;
  }

  .full-width {
    grid-column: auto;
  }
}
</style>