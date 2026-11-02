<template>
  <HeaderBar />

  <div class="forgot-password-page d-flex flex-column align-items-center py-5">
    <h2 class="text-center mb-5 fw-semibold">Forgot Password</h2>

    <form @submit.prevent="submitEmail" novalidate class="forgot-password-form">
      <div class="row g-4 form-section">
        <div class="col-12">
          <label class="form-label">Enter your registered email *</label>
          <input v-model="email" type="email" class="form-control" required />
        </div>
      </div>

      <div class="text-center mt-5">
        <button type="submit" class="btn btn-primary px-4" :disabled="!email">
          Send OTP
        </button>
      </div>

      <div class="text-center mt-4">
        <p class="mb-0 text-muted d-flex justify-content-center">
          Remembered your password?&nbsp;
          <a href="/user/login" class="sign-in-link text-decoration-underline mt-0">Sign In</a>
        </p>
      </div>

      <div class="text-center mt-3">
        <router-link to="/user/login" class="text-decoration-none back-link">
            Back to Login
        </router-link>
      </div>
      
    </form>
  </div>
</template>

<script setup>
import { ref, onUnmounted } from 'vue'
import { useForgotPasswordStore } from '../store/forgotPassword'
import { useRouter } from 'vue-router'
import { toast } from 'vue3-toastify'
import HeaderBar from '../components/HeaderBar.vue'; 

const email = ref('')
const router = useRouter()
const forgotPassword = useForgotPasswordStore()

const submitEmail = async () => {
  if (!email.value) {
    toast.error('Please enter your email address.');
    return;
  }
  
  try {
    await forgotPassword.startForgotPassword(email.value)
    toast.success('OTP sent to your email')
    router.push('/user/forgot-password/verify-otp')
  } catch (error) {
    console.error('Error in submitEmail: ', error)
      toast.error(error.response?.data?.message || 'Error sending OTP')
  }
};
</script>

<style scoped>
.forgot-password-page {
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
  width: 100%;
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

.sign-in-link {
    color: #6366f1;
    font-weight: 600;
}


@media (max-width: 768px) {
  .forgot-password-form {
    padding: 2rem 1.5rem;
    width: 95%;
  }
  .forgot-password-page h2 {
    font-size: 1.8rem;
    margin-bottom: 2.5rem !important;
  }
}
</style>