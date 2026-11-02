<template>
  <HeaderBar />

  <div class="reset-password-page d-flex flex-column align-items-center py-5">
    <h2 class="text-center mb-5 fw-semibold">Set a New Password</h2>

    <div v-if="flowExpired" class="forgot-password-form flow-expired-message text-center">
        <h3 class="text-danger mb-3 fw-bold">Session Expiring...</h3>
        <p class="mb-4 text-secondary">
          {{ redirectErrorMessage }}
        </p>
        <p class="text-primary fw-semibold">
            Redirecting to Login in <span class="text-danger fw-bold">{{ redirectTimer }}</span> seconds...
        </p>
    </div>
    
    <form v-else @submit.prevent="resetPassword" novalidate class="forgot-password-form">

      <div class="form-section">
        <label class="form-label d-flex align-items-center gap-2">
          New Password *
          <div 
            class="password-hint-icon" 
            :title="PASSWORD_PATTERN_HINT"
          >
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-question-circle" viewBox="0 0 16 16">
              <path d="M8 15A7 7 0 1 1 8 1a7 7 0 0 1 0 14m0 1A8 8 0 1 0 8 0a8 8 0 0 0 0 16"/>
              <path d="M5.255 5.786a.237.237 0 0 0 .241.247h.825c.138 0 .248.113.266.25.09.656.54 1.134 1.2 1.45.517.245.88.618.88 1.092 0 .818-.363 1.34-1.198 1.34-.686 0-1.314-.343-1.422-.968-.073-.448.337-.791.758-.791h.563c.371 0 .614-.373.443-.746a1.64 1.64 0 0 0-.585-.453c-.784-.343-1.485-.76-1.485-1.928 0-1.09.91-1.782 1.666-1.782.823 0 1.212.555 1.212 1.572 0 .546-.388.941-.758 1.113zM8 12.5a1 1 0 1 0 0-2 1 1 0 0 0 0 2"/>
            </svg>
          </div>
        </label>
        <div class="input-group mb-3">
          <input 
            v-model="password" 
            :type="passwordType" 
            class="form-control" 
            required 
          />
          <button 
            class="btn btn-outline-secondary password-toggle-btn" 
            type="button" 
            @click="togglePasswordVisibility('password')"
          >
            <svg v-if="passwordType !== 'password'" xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-eye-slash-fill" viewBox="0 0 16 16">
              <path d="m10.79 12.912-1.614-1.615a3.5 3.5 0 0 1-4.38-4.38L2.91 3.209A8 8 0 0 0 13.792 10.79zm-4.229-4.229a4.5 4.5 0 0 1 4.301-5.698l-2.868 2.869a1.5 1.5 0 0 0-2.866 2.868l-2.868 2.868a8 8 0 0 0 10.82-10.82zm2.083-2.613a4.5 4.5 0 0 1 4.298 5.694l-2.867 2.867a1.5 1.5 0 0 0-2.867-2.867l-2.867-2.868a4.5 4.5 0 0 1 4.299-5.694z"/>
            </svg>
            <svg v-else xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-eye-fill" viewBox="0 0 16 16">
              <path d="M10.5 8a2.5 2.5 0 1 1-5 0 2.5 2.5 0 0 1 5 0"/>
              <path d="M0 8s3-5.5 8-5.5S16 8 16 8s-3 5.5-8 5.5S0 8 0 8m8 3.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7"/>
            </svg>
          </button>
        </div>
      </div>

      <div class="form-section">
        <label class="form-label">Confirm Password *</label>
        <div class="input-group mb-4">
          <input 
            v-model="confirm" 
            :type="confirmPasswordType" 
            class="form-control" 
            required 
          />
          <button 
            class="btn btn-outline-secondary password-toggle-btn" 
            type="button" 
            @click="togglePasswordVisibility('confirm')"
          >
            <svg v-if="confirmPasswordType !== 'password'" xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-eye-slash-fill" viewBox="0 0 16 16">
              <path d="m10.79 12.912-1.614-1.615a3.5 3.5 0 0 1-4.38-4.38L2.91 3.209A8 8 0 0 0 13.792 10.79zm-4.229-4.229a4.5 4.5 0 0 1 4.301-5.698l-2.868 2.869a1.5 1.5 0 0 0-2.866 2.868l-2.868 2.868a8 8 0 0 0 10.82-10.82zm2.083-2.613a4.5 4.5 0 0 1 4.298 5.694l-2.867 2.867a1.5 1.5 0 0 0-2.867-2.867l-2.867-2.868a4.5 4.5 0 0 1 4.299-5.694z"/>
            </svg>
            <svg v-else xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-eye-fill" viewBox="0 0 16 16">
              <path d="M10.5 8a2.5 2.5 0 1 1-5 0 2.5 2.5 0 0 1 5 0"/>
              <path d="M0 8s3-5.5 8-5.5S16 8 16 8s-3 5.5-8 5.5S0 8 0 8m8 3.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7"/>
            </svg>
          </button>
        </div>
      </div>
      
      <button 
        class="btn btn-primary w-100" 
        :disabled="!isFormValid"
      >
        Change Password
      </button>
      
      <div class="text-center mt-3">
        <router-link to="/user/login" class="text-decoration-none back-link" @click="resetAll">
            Back to Login
        </router-link>
      </div>
    </form>
  </div>
</template>

<script setup>
import { ref, computed, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useForgotPasswordStore } from '../store/forgotPassword'
import { toast } from 'vue3-toastify'
import HeaderBar from '../components/HeaderBar.vue';

const password = ref('')
const confirm = ref('')
const forgot = useForgotPasswordStore()
const router = useRouter()

const flowExpired = ref(false);
const redirectTimer = ref(5);
const redirectErrorMessage = ref('');

let redirectInterval;

const PASSWORD_PATTERN_REGEX = /^(?=.*[a-z])(?=.*[A-Z])(?=.*\d)(?=.*[@$!%*?&])[A-Za-z\d@$!%*?&]{8,}$/;
const PASSWORD_PATTERN_HINT = 'Password must be at least 8 characters long and include: 1 uppercase letter, 1 lowercase letter, 1 number, and 1 special character (@$!%*?&).';

const passwordType = ref('password');
const confirmPasswordType = ref('password');

const resetAll = () => {
  forgot.resetFlow();
  if (redirectInterval) {
      clearInterval(redirectInterval);
  }
};

const handleFlowExpired = () => {
  flowExpired.value = true;
  
  redirectInterval = setInterval(() => {
    redirectTimer.value--;
    if (redirectTimer.value <= 0) {
      clearInterval(redirectInterval);
      router.push('/user/login');
    }
  }, 1000);
};

const togglePasswordVisibility = (field) => {
  if (field === 'password') {
    passwordType.value = passwordType.value === 'password' ? 'text' : 'password';
  } else if (field === 'confirm') {
    confirmPasswordType.value = confirmPasswordType.value === 'password' ? 'text' : 'password';
  }
};

const isPasswordStrong = computed(() => {
  return PASSWORD_PATTERN_REGEX.test(password.value);
});

const isFormValid = computed(() => {
  return password.value && confirm.value && password.value === confirm.value && isPasswordStrong.value;
});

const resetPassword = async () => {
  if (password.value !== confirm.value) {
    toast.error('Passwords do not match');
    return;
  }
  
  if (!isPasswordStrong.value) {
     toast.error(PASSWORD_PATTERN_HINT);
     return;
  }

  try {
    await forgot.resetPassword(password.value)
    toast.success('Password changed successfully')
    router.push('/user/login')
  } catch (error) {
    if (error.redirect) {
      redirectErrorMessage.value = error.message || 'The password reset flow has expired. Redirecting to login...';
      toast.error('Password reset flow expired. Redirecting to login...');
      handleFlowExpired();
    } else {
      toast.error(error.response?.data?.message || 'Error resetting password')
    }
  }
}

onUnmounted(() => {
    if (redirectInterval) {
        clearInterval(redirectInterval);
    }
});
</script>

<style scoped>
.reset-password-page {
  background-color: #fcfcfd; 
  min-height: 100vh;
  padding-bottom: 5rem !important; 
}

.forgot-password-form {
  width: 90%;
  max-width: 500px;
  background: white;
  border-radius: 1rem;
  padding: 3rem; 
  box-shadow: 0 10px 25px rgba(0, 0, 0, 0.05); 
}

.form-section {
    margin-top: 1.5rem; 
}
.forgot-password-form .form-section:first-of-type {
    margin-top: 0;
}

.form-control {
  border-color: #e2e8f0; 
  border-radius: 0.5rem;
  transition: border-color 0.15s ease-in-out, box-shadow 0.15s ease-in-out;
}

.form-control:focus {
  border-color: #6366f1; 
  box-shadow: 0 0 0 0.25rem rgba(99, 102, 241, 0.25);
}

label {
  font-weight: 600 !important; 
  margin-bottom: 0.4rem;
  font-size: 0.95rem; 
  color: #333;
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

.password-toggle-btn {
  border-color: #e2e8f0 !important;
  background-color: white !important;
  border-left: none !important;
  color: #6b7280;
}
.password-toggle-btn:hover {
  background-color: #f3f4f6 !important;
}
.password-hint-icon {
  color: #6b7280;
  cursor: help;
}

.back-link {
    color: #6366f1;
    font-weight: 500;
    font-size: 0.95rem;
}


@media (max-width: 768px) {
  .forgot-password-form {
    padding: 2rem 1.5rem;
    width: 95%;
  }
  .reset-password-page h2 {
    font-size: 1.8rem;
    margin-bottom: 2.5rem !important;
  }
}
</style>