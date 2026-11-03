import { defineStore } from 'pinia';
import { apiClient } from '../api/http';
import { apiRoutes } from '../api/routes';

export interface UserProfile {
  name?: string;
  email?: string;
  display_picture?: string | null;
  [key: string]: unknown;
}

export const useUserStore = defineStore('user', {
  state: () => ({
    user: null as UserProfile | null,
  }),

  actions: {
    async fetchUserProfile() {
      const response = await apiClient.get<UserProfile>(
        `${apiRoutes.user}/profile`,
      );

      this.user = response.data;
      return this.user;
    },

    clearProfile() {
      this.user = null;
    },
  },
});