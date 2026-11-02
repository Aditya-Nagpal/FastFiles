import { useAuthStore } from './auth';
import { useUserStore } from './user';
import { useFileStore } from './file';
import { useForgotPasswordStore } from './forgotPassword';

export const resetAllStores = () => {
  useAuthStore().$reset();
  useFileStore().$reset();
  useUserStore().$reset();
  useForgotPasswordStore().$reset();
};

export const resetLocalStorage = () => {
  localStorage.removeItem('accessToken');
  localStorage.removeItem('userId');
};