import { createRouter, createWebHistory } from 'vue-router';
ForgotPasswordEmail
import SignIn from '../views/SignIn.vue';
import SignUp from '../views/SignUp.vue';
import Profile from '../views/Profile.vue';
import ForgotPasswordEmail from '../views/ForgotPasswordEmail.vue';
import VerifyOtp from '../views/VerifyOtp.vue';
import ResetPassword from '../views/ResetPassword.vue';
import Home from '../views/Home.vue';
import { useAuthStore } from '../store/auth';
import PUBLIC_PAGES from './publicPages';

const routes = [
  { path: '/user/forgot-password/reset-password', component: ResetPassword },
  { path: '/user/forgot-password/verify-otp', component: VerifyOtp },
  { path: '/user/forgot-password', component: ForgotPasswordEmail },
  { path: '/user/login', component: SignIn },
  { path: '/user/signup', component: SignUp },
  { path: '/user/profile', component: Profile },
  { path: '/', component: Home }
];

const publicPages = PUBLIC_PAGES;

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
});

router.beforeEach(async (to) => {
  const auth = useAuthStore();

  const isPublic = publicPages.includes(to.path);
  const accessToken = auth.accessToken;

  if(!accessToken && !isPublic){
    return '/user/login';
  }

  if(accessToken){
    try {
      await auth.checkAuth();
      if(isPublic){
        return '/';
      }
    } catch (error) {
      return '/user/login';
    }
  }
});

export default router;