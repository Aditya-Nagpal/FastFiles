<template>
  <form @submit.prevent="handleSignUp" novalidate class="signup-form">
      
      <!-- Name (Row 1) -->
      <div class="row g-4 form-section">
        <div class="col-md-6">
          <label class="form-label" for="firstName">First Name *</label>
          <input v-model="form.firstName" type="text" class="form-control" required id="firstName" />
        </div>
        <div class="col-md-6">
          <label class="form-label" for="lastName">Last Name *</label>
          <input v-model="form.lastName" type="text" class="form-control" required id="lastName" />
        </div>
      </div>

      <!-- Email (Row 2) -->
      <div class="row g-4 form-section">
        <div class="col-md-6">
          <label class="form-label" for="email">Email *</label>
          <input v-model="form.email" type="email" class="form-control" required id="email" />
        </div>
        <div class="col-md-6">
          <label class="form-label" for="alternateEmail">Alternate Email</label>
          <input v-model="form.alternateEmail" type="email" class="form-control" id="alternateEmail" />
        </div>
      </div>

      <!-- Contact + Country (Row 3) -->
      <div class="row g-4 form-section">
        <div class="col-md-12">
          <label class="form-label" for="contactNumber">Contact Number *</label>
          <div class="input-group">
            <button
              class="btn btn-outline-secondary d-flex align-items-center gap-2"
              type="button"
              data-bs-toggle="dropdown"
              aria-expanded="false"
            >
              <img :src="selectedCountry.flag" alt="flag" width="22" height="15" />
              <span>{{ selectedCountry.code }}</span>
            </button>
            <ul class="dropdown-menu" style="max-height: 300px; overflow-y: auto;">
              <li
                v-for="country in countries"
                :key="country.name"
                @click="selectCountry(country)"
                class="dropdown-item d-flex align-items-center gap-2"
              >
                <img :src="country.flag" alt="flag" width="22" height="15" />
                <span>{{ country.name }} ({{ country.code }})</span>
              </li>
            </ul>
            <input
              v-model="form.contactNumber"
              type="tel"
              class="form-control"
              maxlength="10"
              @keypress="isNumberKey"
              @input="formatContactNumber"
              required
              id="contactNumber"
            />
          </div>
        </div>
      </div>

      <!-- Gender + DOB (Row 4) -->
      <div class="row g-4 form-section">
        <div class="col-md-6">
          <label class="form-label" for="gender">Gender *</label>
          <select v-model="form.gender" class="form-select" required id="gender">
            <option disabled value="">Select gender</option>
            <option v-for="g in genderOptions" :key="g" :value="g">{{ processGenderKey(g) }}</option>
          </select>
        </div>

        <div class="col-md-6">
          <label class="form-label" for="DOB">Date Of Birth *</label>
          <input 
              v-model="form.dob" 
              type="date" 
              class="form-control" 
              required 
              placeholder="dd/mm/yyyy"
              id="DOB"
            />
        </div>
      </div>

      <!-- Timezone + Plan (Row 5) -->
      <div class="row g-4 form-section">
        <div class="col-md-6">
          <label class="form-label" for="timezone">Timezone *</label>
          <select v-model="form.timezone" class="form-select" required id="timezone">
            <option disabled value="">Select timezone</option>
            <option v-for="t in timezones" :key="t" :value="t">{{ t }}</option>
          </select>
        </div>

        <div class="col-md-6">
          <label class="form-label" for="plan">Plan *</label>
          <select v-model="form.plan" class="form-select" required id="plan">
            <option disabled value="">Select plan</option>
            <option v-for="p in planOptions" :key="p" :value="p">{{ p }}</option>
          </select>
        </div>
      </div>

      <!-- About (Row 6) -->
      <div class="row g-4 form-section">
        <div class="col-12">
          <label class="form-label" for="about">About</label>
          <textarea
            v-model="form.about"
            rows="3"
            class="form-control"
            placeholder="Tell us about yourself..."
            id="about"
            :maxlength="MAX_ABOUT_CHARS"
          ></textarea>
          <div class="char-count-display">
            {{ form.about.length }} / {{ MAX_ABOUT_CHARS }} characters used
          </div>
        </div>
      </div>

      <!-- Passwords (Row 7) -->
      <div class="row g-4 form-section">
        <div class="col-md-6">
          <div class="d-flex align-items-center mb-2">
            <label class="form-label mb-0" for="password">Password *</label>
            <span 
                class="password-info-icon ms-2"
                :title="PASSWORD_REQUIREMENTS.message"
            >
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-question-circle" viewBox="0 0 16 16">
                  <path d="M8 15A7 7 0 1 1 8 1a7 7 0 0 1 0 14zm0 1A8 8 0 1 0 8 0a8 8 0 0 0 0 16z"/>
                  <path d="M5.31 9.497c-.075.051-.102.134-.102.219a.443.443 0 0 0 .102.22c.162.115.421.192.83.256.408.062.835.093 1.258.093 1.258 0 2.22-.387 2.923-1.166.703-.778 1.055-1.92 1.055-3.41 0-1.042-.32-1.907-.96-2.595C10.603 3.978 9.614 3.5 8.358 3.5c-1.29 0-2.316.398-3.076 1.193C4.522 5.485 4.148 6.57 4.148 7.842c0 .285.04.536.12.753.08.216.195.398.345.546zM8 4.792c.69 0 1.28.21 1.77.63.49.42.735 1.07.735 1.942 0 1.15-.365 2.073-1.095 2.77C9.07 10.45 8.37 10.792 7.425 10.792c-.77 0-1.445-.257-2.025-.773-.58-.516-.87-1.287-.87-2.313 0-1.353.44-2.39 1.32-3.11.88-.72 2.01-1.08 3.39-1.08z"/>
                </svg>
            </span>
          </div>
          <div class="input-group">
            <input 
              v-model="form.password" 
              :type="passwordType" 
              class="form-control" 
              required
              id="password"
            />
            <button 
                class="btn btn-outline-secondary password-toggle-btn" 
                type="button" 
                @click="togglePasswordVisibility('password')"
            >
                <span v-if="passwordType === 'password'">
                    <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-eye" viewBox="0 0 16 16">
                        <path d="M16 8s-3-5.5-8-5.5S0 8 0 8s3 5.5 8 5.5S16 8 16 8M1.173 8a13.133 13.133 0 0 1 1.66-2.043C4.12 4.668 5.88 3.5 8 3.5c2.12 0 3.879 1.168 5.168 2.457A13.134 13.134 0 0 1 14.828 8c-.058.156-.128.307-.208.452-.259.45-.588.859-1.02 1.173-1.011.776-2.096 1.487-3.344 1.777C9.28 11.516 8.653 11.5 8 11.5c-1.325 0-2.618-.453-3.714-1.258C2.969 9.531 1.957 8.52 1.173 8z"/>
                        <path d="M8 5.5a2.5 2.5 0 1 0 0 5 2.5 2.5 0 0 0 0-5M4.5 8a3.5 3.5 0 1 1 7 0 3.5 3.5 0 0 1-7 0"/>
                    </svg>
                </span>
                <span v-else>
                    <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-eye-slash" viewBox="0 0 16 16">
                        <path d="M10.79 12.912l-1.614-1.615a3.5 3.5 0 0 1-4.743-4.743L3.102 3.898A13.155 13.155 0 0 0 1.173 8c.328 1.056.887 2.052 1.579 2.924l-.121.12c-.22.22-.39.46-.51.72-.25.56-.401 1.252-.387 1.839.01.52.126 1.05.342 1.55l.135.31c-.21.49.52.92.89 1.27l.14.12c.38.35.83.61 1.32.78l.2.06c.55.16 1.12.24 1.69.24.44 0 .89-.04 1.32-.12l.2-.04c-.54-.15-1.04-.42-1.48-.82l-.14-.14c-.39-.37-.7-.79-.93-1.25l.12-.25c.2-.42.34-.87.4-1.35.06-.47.05-.95-.03-1.42.06.48-.03 1.02-.31 1.51-.23.41-.53.79-.9.95L8 11.23l2.79 2.79 1.41-1.42L12.21 11.5z"/>
                        <path d="M8 5.5a2.5 2.5 0 0 0-2.5 2.5c0 .04.004.08.008.118l3.642 3.642a2.5 2.5 0 0 0-3.5-3.5z"/>
                        <path d="M12.912 10.79l-1.615-1.614a3.5 3.5 0 0 0-4.743-4.743L3.898 3.102A13.155 13.155 0 0 0 8 1.173c1.056.328 2.052.887 2.924 1.579l.121-.12c-.22-.22-.39-.46-.51-.72-.25-.56-.401 1.252-.387 1.839-.01-.52-.126-1.05-.342-1.55l-.135-.31c-.21-.49-.52-.92-.89-1.27l-.14-.12c-.38-.35-.83-.61-1.32-.78l-.2-.06c-.55-.16-1.12-.24-1.69-.24-.44 0-.89.04-1.32.12l-.2.04c-.54.15-1.04.42-1.48.82l-.14.14c-.39.37-.7.79-.93 1.25L4.5 4.35 1.17 1.02 0 2.44 1.42 3.85 2.83 5.26 11.21 13.64 12.63 12.23 11.22 10.82z"/>
                    </svg>
                </span>
            </button>
          </div>
        </div>
        <div class="col-md-6">
          <label class="form-label" for="confirmPassword">Confirm Password *</label>
          <div class="input-group">
            <input 
              v-model="form.confirmPassword" 
              :type="confirmPasswordType" 
              class="form-control" 
              required 
              id="confirmPassword"
            />
            <button 
                class="btn btn-outline-secondary password-toggle-btn" 
                type="button" 
                @click="togglePasswordVisibility('confirmPassword')"
            >
                <span v-if="confirmPasswordType === 'password'">
                    <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-eye" viewBox="0 0 16 16">
                        <path d="M16 8s-3-5.5-8-5.5S0 8 0 8s3 5.5 8 5.5S16 8 16 8M1.173 8a13.133 13.133 0 0 1 1.66-2.043C4.12 4.668 5.88 3.5 8 3.5c2.12 0 3.879 1.168 5.168 2.457A13.134 13.134 0 0 1 14.828 8c-.058.156-.128.307-.208.452-.259.45-.588.859-1.02 1.173-1.011.776-2.096 1.487-3.344 1.777C9.28 11.516 8.653 11.5 8 11.5c-1.325 0-2.618-.453-3.714-1.258C2.969 9.531 1.957 8.52 1.173 8z"/>
                        <path d="M8 5.5a2.5 2.5 0 1 0 0 5 2.5 2.5 0 0 0 0-5M4.5 8a3.5 3.5 0 1 1 7 0 3.5 3.5 0 0 1-7 0"/>
                    </svg>
                </span>
                <span v-else>
                    <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-eye-slash" viewBox="0 0 16 16">
                        <path d="M10.79 12.912l-1.614-1.615a3.5 3.5 0 0 1-4.743-4.743L3.102 3.898A13.155 13.155 0 0 0 1.173 8c.328 1.056.887 2.052 1.579 2.924l-.121.12c-.22.22-.39.46-.51.72-.25.56-.401 1.252-.387 1.839.01.52.126 1.05.342 1.55l.135.31c-.21.49-.52.92-.89 1.27l.14.12c-.38.35-.83.61 1.32.78l.2.06c.55.16 1.12.24 1.69.24.44 0 .89-.04 1.32-.12l.2-.04c-.54-.15-1.04-.42-1.48-.82l-.14-.14c-.39-.37-.7-.79-.93-1.25l.12-.25c.2-.42.34-.87.4-1.35.06-.47.05-.95-.03-1.42.06.48-.03 1.02-.31 1.51-.23.41-.53.79-.9.95L8 11.23l2.79 2.79 1.41-1.42L12.21 11.5z"/>
                        <path d="M8 5.5a2.5 2.5 0 0 0-2.5 2.5c0 .04.004.08.008.118l3.642 3.642a2.5 2.5 0 0 0-3.5-3.5z"/>
                        <path d="M12.912 10.79l-1.615-1.614a3.5 3.5 0 0 0-4.743-4.743L3.898 3.102A13.155 13.155 0 0 0 8 1.173c1.056.328 2.052.887 2.924 1.579l.121-.12c-.22-.22-.39-.46-.51-.72-.25-.56-.401 1.252-.387 1.839-.01-.52-.126-1.05-.342-1.55l-.135-.31c-.21-.49-.52-.92-.89-1.27l-.14-.12c-.38-.35-.83-.61-1.32-.78l-.2-.06c-.55-.16-1.12-.24-1.69-.24-.44 0-.89.04-1.32.12l-.2.04c-.54.15-1.04.42-1.48.82l-.14.14c-.39.37-.7.79-.93 1.25L4.5 4.35 1.17 1.02 0 2.44 1.42 3.85 2.83 5.26 11.21 13.64 12.63 12.23 11.22 10.82z"/>
                    </svg>
                </span>
            </button>
          </div>
        </div>
      </div>

      <!-- Terms (Row 8) -->
      <div class="form-check terms-section">
        <input
          v-model="form.termsAndPrivacy"
          type="checkbox"
          class="form-check-input"
          required
          id="terms"
        />
        <label for="terms" class="form-check-label d-flex align-items-start">
            I agree to the <a href="#" class="text-decoration-underline">Terms and Privacy Policy</a>
        </label>
      </div>

      <!-- Submit (Row 9) -->
      <div class="text-center mt-5">
        <button type="submit" class="btn btn-primary px-4" :disabled="!isFormValid">
          Sign Up
        </button>
      </div>
      
      <!-- Added Sign In Link (New Row) -->
      <div class="text-center mt-3">
        <p class="mb-0 text-muted d-flex justify-content-center">
          Account exists?&nbsp;
          <a href="/user/login" class="sign-in-link text-decoration-underline mt-0">Sign In</a>
        </p>
      </div>
    </form>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue';
import { useAuthStore } from '../store/auth';
import { useRouter } from 'vue-router';
import { toast } from 'vue3-toastify';

const auth = useAuthStore();
const router = useRouter();

const MAX_ABOUT_CHARS = 500;

const PASSWORD_REQUIREMENTS = {
    regex: /^(?=.*[a-z])(?=.*[A-Z])(?=.*\d)(?=.*[@$!%*?&])[A-Za-z\d@$!%*?&]{8,}$/,
    message: "Minimum 8 characters, including 1 uppercase, 1 lowercase, 1 number, and 1 special character (@$!%*?&)."
};

const passwordType = ref('password');
const confirmPasswordType = ref('password');

const togglePasswordVisibility = (field) => {
  if (field === 'password') {
    passwordType.value = passwordType.value === 'password' ? 'text' : 'password';
  } else if (field === 'confirmPassword') {
    confirmPasswordType.value = confirmPasswordType.value === 'password' ? 'text' : 'password';
  }
};

const isNumberKey = (evt) => {
  const charCode = (evt.which) ? evt.which : evt.keyCode;
  if (charCode > 31 && (charCode < 48 || charCode > 57)) {
    evt.preventDefault();
  }
  return true;
};

const formatContactNumber = () => {
    let cleaned = form.value.contactNumber.replace(/\D/g, '');
    form.value.contactNumber = cleaned.slice(0, 10);
};

const processGenderKey = (g) => {
  if(g !== '') {
    let G = g.split('_').map((word) => word.charAt(0).toUpperCase() + word.slice(1));
    return G.join(' ');
  }
  return '';
};

const processNameKey = (n) => {
  if(n) {
    let N = n.trim();
    return N[0].toUpperCase() + N.slice(1).toLowerCase();
  }
  return '';
};

const validateEmail = (text) => {
  const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  return emailPattern.test(text);
};

const form = ref({
  firstName: '',
  lastName: '',
  email: '',
  alternateEmail: '',
  contactNumber: '',
  gender: '',
  dob: '',
  timezone: '',
  about: '',
  plan: '',
  password: '',
  confirmPassword: '',
  termsAndPrivacy: false
});

const genderOptions = ['male', 'female', 'other', 'prefer_not_to_say'];
const planOptions = ['free', 'pro', 'business'];
const countries = ref([]);
const selectedCountry = ref({ name: 'India', code: '+91', flag: 'https://flagcdn.com/w40/in.png' });
const timezones = ref([]);

onMounted(async () => {
  try {
    const res = await fetch('https://restcountries.com/v3.1/all?fields=name,idd,flags');
    const data = await res.json();
    countries.value = data
      .map(country => ({
        name: country.name.common,
        code: country.idd?.root
          ? `${country.idd.root}${country.idd?.suffixes ? country.idd.suffixes[0] : ''}`
          : '',
        flag: country.flags.png
      }))
      .filter(country => country.code)
      .sort((a, b) => a.name.localeCompare(b.name));
  } catch (error) {
    console.error('Error fetching countries:', error);
  }

  timezones.value = Intl.supportedValuesOf('timeZone').sort();
});

const selectCountry = country => {
  selectedCountry.value = country;
};

const isContactNumberValid = computed(() => {
  const contact = form.value.contactNumber;
  return contact && /^\d+$/.test(contact) && contact.length === 10;
});

const isPasswordStrong = computed(() => {
  return PASSWORD_REQUIREMENTS.regex.test(form.value.password);
});

const isFormValid = computed(() => {
  return (
    form.value.firstName &&
    form.value.email &&
    isContactNumberValid.value &&
    form.value.gender &&
    form.value.dob &&
    form.value.timezone &&
    form.value.plan &&
    form.value.password &&
    form.value.password === form.value.confirmPassword &&
    form.value.termsAndPrivacy
  );
});

const handleSignUp = async () => {
  if (form.value.password !== form.value.confirmPassword) {
    toast.error('Passwords do not match');
    return;
  }

  if(!validateEmail(form.value.email)) {
    toast.error('Invalid Email');
    return;
  }

  if (!isPasswordStrong.value) {
    toast.error(PASSWORD_REQUIREMENTS.message);
    return;
  }

  form.value.firstName = processNameKey(form.value.firstName);
  form.value.lastName = processNameKey(form.value.lastName);
  form.value.contactNumber = form.value.contactNumber.trim();

  const payload = {
    ...form.value,
    name: `${form.value.firstName} ${form.value.lastName}`,
    country: selectedCountry.value.name,
    contactNumber: `${selectedCountry.value.code}${form.value.contactNumber}`
  };

  delete payload.firstName;
  delete payload.lastName;
  delete payload.confirmPassword;

  try {
    await auth.signUp(payload);
    toast.success('Signed Up Successfully!');
    router.push('/user/login');
  } catch (error) {
    console.error('Sign up error:', error);
    toast.error(error.response?.data?.message || 'Sign-up failed');
  }
};
</script>

<style scoped>
.signup-page {
  background-color: #fcfcfd; 
  min-height: 100vh;
  padding-bottom: 5rem !important; 
}

.signup-form {
  width: 90%;
  max-width: 950px;
  background: white;
  border-radius: 1rem;
  padding: 2rem 4rem; 
  box-shadow: 0 10px 25px rgba(0, 0, 0, 0.05); 
}

.form-section {
    margin-top: 1.5rem;
}

.signup-form .form-section:first-of-type {
    margin-top: 0;
}

.char-count-display {
    text-align: right;
    font-size: 0.85rem;
    color: #6b7280;
    margin-top: 0.25rem;
    padding-right: 0.25rem;
}

.password-info-icon {
    color: #6366f1;
    cursor: help;
    transition: transform 0.1s ease; 
    display: inline-block;
}

.password-info-icon:hover {
    color: #4f46e5;
    transform: scale(1.1);
}

.input-group input.form-control {
    border-radius: 0.5rem 0 0 0.5rem !important;
}

#contactNumber {
  border-radius: 0 0.5rem 0.5rem 0 !important;
}

.password-toggle-btn {
  border-color: #e2e8f0 !important;
  background-color: #f8f8f8 !important;
  border-left: 0 !important;
  border-radius: 0 0.5rem 0.5rem 0 !important;
  padding: 0.375rem 0.75rem;
}

.password-toggle-btn:hover,
.password-toggle-btn:focus,
.password-toggle-btn:active {
  background-color: #f8f8f8 !important;
  border-color: #e2e8f0 !important;
  box-shadow: none !important;
  color: #333 !important;
}

.terms-section {
    margin-top: 2rem !important; 
    padding-left: 0 !important;
    display: flex;
    align-items: center;
}

.terms-section .form-check-input {
    margin-top: 0;
    margin-right: 0.5rem; 
    margin-left: 0;
}

.terms-section .form-check-label {
    display: flex; 
    align-items: center;
    gap: 0.25rem;
    font-weight: 500 !important;
    font-size: 0.95rem;
    color: #333;
    line-height: 1.2; 
    margin-bottom: 0;
    padding-left: 0;
}

.terms-section a {
    margin-top: 0;
    white-space: nowrap;
}

.form-control,
.form-select,
textarea {
  width: 100%;
  border-color: #e2e8f0; 
  border-radius: 0.5rem;
  transition: border-color 0.15s ease-in-out, box-shadow 0.15s ease-in-out;
}

.form-control:focus,
.form-select:focus,
textarea:focus {
  border-color: #6366f1; 
  box-shadow: 0 0 0 0.25rem rgba(99, 102, 241, 0.25);
}

label {
  font-weight: 600 !important; 
  margin-bottom: 0.4rem;
  font-size: 0.95rem; 
  color: #333;
}

.input-group-text {
  background-color: white; 
  border-left: 0;
  border-color: #e2e8f0;
  border-radius: 0 0.5rem 0.5rem 0;
  color: #666; 
}

.input-group .btn-outline-secondary {
  border-right: 0;
  border-color: #e2e8f0;
  background-color: #f8f8f8; 
  padding-left: 0.75rem;
  padding-right: 0.75rem;
  border-radius: 0.5rem 0 0 0.5rem;
}

.input-group .btn-outline-secondary:hover,
.input-group .btn-outline-secondary:focus,
.input-group .btn-outline-secondary:active {
  background-color: #f8f8f8 !important; 
  border-color: #e2e8f0 !important; 
  color: #333;
  box-shadow: none !important;
}

.dropdown-menu {
  border-color: #e2e8f0;
  border-radius: 0.5rem;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.dropdown-item:hover {
  background-color: #f3f4f6;
}

.btn-primary {
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1); 
  border-radius: 0.5rem;
  padding: 0.6rem 1.8rem;
  font-weight: 600;
  background-color: #6366f1;
  border-color: #6366f1;
}

.btn-primary:hover {
  background-color: #4f46e5;
  border-color: #4f46e5;
}

.sign-in-link {
    color: #6366f1;
    font-weight: 600;
}


@media (max-width: 768px) {
  .signup-form {
    padding: 2rem 1.5rem;
    width: 95%;
  }
  .signup-page h2 {
    font-size: 1.8rem;
    margin-bottom: 2.5rem !important;
  }
}
</style>