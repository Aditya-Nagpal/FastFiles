<template>
  <div class="signin-page d-flex flex-column align-items-center py-5">
    <h2 class="text-center mb-5 fw-semibold">Sign In to Your FastFiles Account</h2>

    <form @submit.prevent="handleSignIn" novalidate class="signin-form">

        <div class="row g-4 form-section">
            <div class="col-12 mt-0">
            <label class="form-label">Email *</label>
            <input v-model="form.email" type="email" class="form-control" required />
            </div>
        </div>

        <div class="row g-4 form-section">
            <div class="col-12 mt-0">
            <label class="form-label">Password *</label>
            <div class="input-group">
                <input 
                v-model="form.password" 
                :type="passwordType" 
                class="form-control" 
                required 
                />
                <button 
                    class="btn btn-outline-secondary password-toggle-btn" 
                    type="button" 
                    @click="togglePasswordVisibility"
                >
                    <span v-if="passwordType === 'password'">
                        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-eye" viewBox="0 0 16 16">
                            <path d="M16 8s-3-5.5-8-5.5S0 8 0 8s3 5.5 8 5.5S16 8 16 8M1.173 8a13.133 13.133 0 0 1 1.66-2.043C4.12 4.668 5.88 3.5 8 3.5c2.12 0 3.879 1.168 5.168 2.457A13.134 13.134 0 0 1 14.828 8c-.058.156-.128.307-.208.452-.259.45-.588.859-1.02 1.173-1.011.776-2.096 1.487-3.344 1.777C9.28 11.516 8.653 11.5 8 11.5c-1.325 0-2.618-.453-3.714-1.258C2.969 9.531 1.957 8.52 1.173 8z"/>
                            <path d="M8 5.5a2.5 2.5 0 1 0 0 5 2.5 2.5 0 0 0 0-5M4.5 8a3.5 3.5 0 1 1 7 0 3.5 3.5 0 0 1-7 0"/>
                        </svg>
                    </span>
                    <span v-else>
                        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-eye-slash" viewBox="0 0 16 16">
                            <path d="M10.79 12.912l-1.614-1.615a3.5 3.5 0 0 1-4.743-4.743L3.102 3.898A13.155 13.155 0 0 0 1.173 8c.328 1.056.887 2.052 1.579 2.924l-.121.12c-.22.22-.39.46-.51.72-.25.56-.401 1.252-.387 1.839.01.52.126 1.05.342 1.55l.135.31c.21.49.52.92.89 1.27l.14.12c.38.35.83.61 1.32.78l.2.06c.55.16 1.12.24 1.69.24.44 0 .89-.04 1.32-.12l.2-.04c-.54-.15-1.04-.42-1.48-.82l.14-.14c-.39-.37-.7-.79-.93-1.25l.12-.25c.2-.42.34-.87.4-1.35.06-.47.05-.95-.03-1.42.06.48-.03 1.02-.31 1.51-.23.41-.53.79-.9.95L8 11.23l2.79 2.79 1.41-1.42L12.21 11.5z"/>
                            <path d="M8 5.5a2.5 2.5 0 0 0-2.5 2.5c0 .04.004.08.008.118l3.642 3.642a2.5 2.5 0 0 0-3.5-3.5z"/>
                            <path d="M12.912 10.79l-1.615-1.614a3.5 3.5 0 0 0-4.743-4.743L3.898 3.102A13.155 13.155 0 0 0 8 1.173c1.056.328 2.052.887 2.924 1.579l.121-.12c-.22-.22-.39-.46-.51-.72-.25-.56-.401 1.252-.387 1.839-.01-.52-.126-1.05-.342-1.55l-.135-.31c-.21-.49-.52-.92-.89-1.27l-.14-.12c-.38-.35-.83-.61-1.32-.78l-.2-.06c-.55-.16-1.12-.24-1.69-.24-.44 0-.89.04-1.32.12l-.2.04c-.54.15-1.04.42-1.48.82l-.14.14c-.39.37-.7.79-.93 1.25L4.5 4.35 1.17 1.02 0 2.44 1.42 3.85 2.83 5.26 11.21 13.64 12.63 12.23 11.22 10.82z"/>
                        </svg>
                    </span>
                </button>
            </div>
            </div>
        </div>

        <div class="d-flex justify-content-center mt-3">
            <a href="/user/forgot-password" class="forgot-password-link">
            Forgot Password?
            </a>
        </div>

        <div class="text-center mt-5">
            <button type="submit" class="btn btn-primary px-4" :disabled="!isFormValid">
            Sign In
            </button>
        </div>

        <div class="text-center mt-4">
            <p class="mb-0 text-muted d-flex justify-content-center">
            Don't have an account?&nbsp;
            <a href="/user/signup" class="sign-up-link text-decoration-underline mt-0">Sign Up</a>
            </p>
        </div>
    </form>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue';
import { useAuthStore } from '../store/auth'; 
import { useUserStore } from '../store/user';
import { useRouter } from 'vue-router';
import { toast } from 'vue3-toastify';

const auth = useAuthStore();
const user = useUserStore();
const router = useRouter();

const form = ref({
  email: '',
  password: '',
});

const passwordType = ref('password');

const togglePasswordVisibility = () => {
  passwordType.value = passwordType.value === 'password' ? 'text' : 'password';
};

const isFormValid = computed(() => {
  return form.value.email.trim() !== '' && form.value.password.trim() !== '';
});

const handleSignIn = async () => {
  if (!isFormValid.value) {
    toast.error('Please enter both email and password.');
    return;
  }

  form.value.email = form.value.email.trim();
  form.value.password = form.value.password.trim();
  
  try {
    await auth.signIn(form.value.email, form.value.password);
    toast.success('Login successful');
    router.push('/');
    try {
        console.log('Fetching user profile ...');
        await user.fetchUserProfile();
        console.log('User profile fetched:', user.user);
    } catch (error) {
        console.error('Error in fetchUserProfile:', error);
    }
  } catch (error) {
    console.error('Error in handleSignIn', error);
    toast.error(error.response.data.message);
    return;
  }
};
</script>

<style scoped>
.signin-page {
  background-color: #fcfcfd; 
  min-height: 100vh;
  padding-bottom: 5rem !important; 
}

.signin-form {
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
.signin-form .form-section:first-of-type {
    margin-top: 0;
}

.form-control,
.form-select,
textarea {
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

.input-group input.form-control {
    border-radius: 0.5rem 0 0 0.5rem !important;
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

.forgot-password-link,
.sign-up-link {
    color: #6366f1;
    font-size: 0.9rem;
    font-weight: 600;
}

@media (max-width: 768px) {
  .signin-form {
    padding: 2rem 1.5rem;
    width: 95%;
  }
  .signin-page h2 {
    font-size: 1.8rem;
    margin-bottom: 2.5rem !important;
  }
}
</style>
