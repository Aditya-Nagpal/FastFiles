import { createRouter, createWebHistory } from 'vue-router';
import ForgotPasswordView from '../views/ForgotPasswordView.vue';
import HomeView from '../views/HomeView.vue';
import ResetPasswordView from '../views/ResetPasswordView.vue';
import SignInView from '../views/SignInView.vue';
import SignUpView from '../views/SignUpView.vue';
import VerifyOtpView from '../views/VerifyOtpView.vue';
import ProfileView from '../views/ProfileView.vue';
import { pinia } from '../stores/pinia';
import { useAuthStore } from '../stores/auth';
import { useUserStore } from '../stores/user';
import StarredView from '../views/StarredView.vue';
import TrashView from '../views/TrashView.vue';

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      component: HomeView,
      meta: { requiresAuth: true },
    },
    { path: '/user/login', name: 'login', component: SignInView },
    { path: '/user/signup', component: SignUpView },
    { path: '/user/forgot-password', component: ForgotPasswordView },
    {
      path: '/user/forgot-password/verify-otp',
      component: VerifyOtpView,
    },
    {
      path: '/user/forgot-password/reset-password',
      component: ResetPasswordView,
    },
    {
      path: '/user/profile',
      name: 'profile',
      component: ProfileView,
      meta: { requiresAuth: true },
    },
    {
      path: '/starred',
      name: 'starred',
      component: StarredView,
      meta: { requiresAuth: true },
    },
    {
      path: '/trash',
      name: 'trash',
      component: TrashView,
      meta: { requiresAuth: true },
    },
  ],
});

router.beforeEach(async (to) => {
  if (!to.meta.requiresAuth) return true;

  const auth = useAuthStore(pinia);
  const user = useUserStore(pinia);

  try {
    await auth.checkAuth();
    return true;
  } catch {
    auth.clearSession();
    user.clearProfile();

    return {
      name: 'login',
      query: { redirect: to.fullPath },
    };
  }
});

export default router;