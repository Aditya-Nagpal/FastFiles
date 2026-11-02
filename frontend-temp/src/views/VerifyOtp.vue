<template>
  <HeaderBar />

  <div class="verify-otp-page d-flex flex-column align-items-center py-5">
    <h2 class="text-center mb-5 fw-semibold">Verify OTP</h2>

    <div v-if="flowExpired" class="forgot-password-form flow-expired-message text-center">
        <h3 class="text-danger mb-3 fw-bold">Session Expiring...</h3>
        <p class="mb-4 text-secondary">
          {{  redirectErrorMessage }}
        </p>
        <p class="text-primary fw-semibold">
            Redirecting to Login in <span class="text-danger fw-bold">{{ redirectTimer }}</span> seconds...
        </p>
    </div>

    <form v-else @submit.prevent="verify" novalidate class="forgot-password-form">

      <div class="row g-4 form-section mb-4">
        <div class="col-12 text-center">
          <label class="form-label">Enter the 6-digit OTP sent to your email</label>
        </div>
      </div>

      <div class="otp-input-container d-flex justify-content-center gap-2 mb-5">
        <input 
          v-for="(digit, index) in otpDigits" 
          :key="index"
          :ref="el => otpRefs[index] = el"
          v-model="otpDigits[index]"
          type="tel"
          maxlength="1"
          class="otp-box form-control text-center"
          @input="handleInput(index)"
          @keydown.backspace="handleKeyDown(index, $event)"
          required
        />
      </div>

      <div class="text-center mb-4">
        <button 
          type="submit" 
          class="btn btn-primary w-100" 
          :disabled="fullOtp.length !== OTP_LENGTH"
        >
          Verify
        </button>
      </div>
      
      <div class="text-center">
        <button
          type="button"
          class="btn btn-outline-secondary resend-btn"
          :disabled="cooldown > 0"
          @click="resend"
        >
          Resend OTP <span v-if="cooldown > 0">({{ cooldown }}s)</span>
        </button>
      </div>

      <div class="text-center mt-3">
        <router-link to="/user/login" class="text-decoration-none back-link" @click="resetAll">
            Back to Login
        </router-link>
      </div>

    </form>
  </div>
</template>

<script setup>
import { ref, onUnmounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { toast } from 'vue3-toastify'
import { useForgotPasswordStore } from '../store/forgotPassword'
import HeaderBar from '../components/HeaderBar.vue'; 

const OTP_LENGTH = 6;
const cooldown = ref(0)
const forgot = useForgotPasswordStore()
const router = useRouter()

const flowExpired = ref(false);
const redirectTimer = ref(5);
const redirectErrorMessage = ref('');

let redirectInterval;

const otpDigits = ref(Array(OTP_LENGTH).fill(''));
const otpRefs = ref([]);
let timer;

const fullOtp = computed(() => otpDigits.value.join(''));

const handleFlowExpired = () => {
  flowExpired.value = true;
  clearInterval(timer);

  redirectInterval = setInterval(() => {
    redirectTimer.value--;
    if (redirectTimer.value <= 0) {
      clearInterval(redirectInterval);
      router.push('/user/login');
    }
  }, 1000);
};

const resetOtp = () => {
  otpDigits.value = Array(OTP_LENGTH).fill('');
  setTimeout(() => {
    otpRefs.value[0]?.focus();
  }, 0);
};

const resetAll = () => {
  resetOtp();
  forgot.resetFlow();
  cooldown.value = 0;
  clearInterval(timer);
};

const handleInput = (index) => {
  let value = otpDigits.value[index];
  
  value = value.replace(/\D/g, '');
  if (value.length > 1) {
    value = value.charAt(0);
  }
  otpDigits.value[index] = value;

  if (value && index < OTP_LENGTH - 1) {
    otpRefs.value[index + 1]?.focus();
  }
};

const handleKeyDown = (index, event) => {
  if (event.key === 'Backspace' && !otpDigits.value[index] && index > 0) {
    otpRefs.value[index - 1]?.focus();
  }
  if (event.key.length === 1 && /\D/.test(event.key)) {
     event.preventDefault();
  }
};

const verify = async () => {
  if (fullOtp.value.length !== OTP_LENGTH) {
    toast.error('Please enter the complete 6-digit OTP.');
    return;
  }
  
  try {
    await forgot.verifyOtp(fullOtp.value)
    toast.success('OTP verified successfully')
    router.push('/user/forgot-password/reset-password')
  } catch (err) {
    if (err.redirect) {
      redirectErrorMessage.value = err.message || 'The verification flow has expired. Redirecting to login...';
      toast.error('Verification flow expired. Redirecting to login...');
      handleFlowExpired();
    } else {
      toast.error(err.response?.data?.message || 'Invalid OTP')
    }
  }

  resetOtp();
};

const resend = async () => {
  try {
    await forgot.resendOtp()
    toast.info('OTP resent')
    startCooldown()
  } catch (err) {
    if (err.redirect) {
      redirectErrorMessage.value = err.message || 'The verification flow has expired. Redirecting to login...';
      toast.error('Verification flow expired. Cannot resend. Redirecting to login...');
      handleFlowExpired();
    } else {
      toast.error(err.response?.data?.message || 'Error resending OTP')
    }
  }
};

function startCooldown() {
  if(timer) clearInterval(timer);
  
  cooldown.value = Number(import.meta.env.VITE_RESEND_OTP_COOLDOWN_SECONDS) || 30;
  timer = setInterval(() => {
    if (cooldown.value > 0) cooldown.value--
    else clearInterval(timer)
  }, 1000)
};

startCooldown(); 

onUnmounted(() => {
    clearInterval(timer);
    if (redirectInterval) {
        clearInterval(redirectInterval);
    }
});
</script>

<style scoped>
.verify-otp-page {
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

.otp-box {
    width: 45px;
    height: 45px;
    font-size: 1.5rem;
    font-weight: 400;
    box-shadow: none !important; 
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

.resend-btn {
    border-color: #e2e8f0;
    color: #6366f1;
    font-weight: 600;
    transition: all 0.2s;
    width: 100%; 
}

.resend-btn:hover:not(:disabled) {
    background-color: #eef2ff;
    color: #4f46e5;
    border-color: #d1d5db;
}

.resend-btn:disabled {
    cursor: not-allowed;
    background-color: white;
    color: #9ca3af;
    border-color: #e2e8f0;
}

@media (max-width: 768px) {
  .forgot-password-form {
    padding: 2rem 1.5rem;
    width: 95%;
  }
  .verify-otp-page h2 {
    font-size: 1.8rem;
    margin-bottom: 2.5rem !important;
  }
  .otp-box {
      width: 40px; 
      height: 40px; 
      font-size: 1.2rem;
  }
}
</style>