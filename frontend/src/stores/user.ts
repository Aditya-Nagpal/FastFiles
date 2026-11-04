import { defineStore } from 'pinia';
import { apiClient } from '../api/http';
import { apiRoutes } from '../api/routes';

export interface UserProfile {
  name?: string;
  email?: string;
  display_picture?: string | null;
  alternate_email?: string | null;
  contact_number?: string | null;
  country?: string | null;
  gender?: string | null;
  timezone?: string | null;
  plan?: string | null;
  about?: string | null;
  dob?: string | null;
  age?: number | null;
  [key: string]: unknown;
}

interface UpdateProfileResponse {
  updatedUser: Partial<UserProfile>;
}

interface UpdateDisplayPictureResponse {
  displayPicture: string | null;
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

    async updateUserProfile(payload: Partial<UserProfile>) {
      const response = await apiClient.patch<UpdateProfileResponse>(
        `${apiRoutes.user}/profile?dp=false`,
        payload,
      );

      if (this.user && response.data.updatedUser) {
        this.user = {
          ...this.user,
          ...response.data.updatedUser,
        };
      }

      return this.user;
    },

    async updateDisplayPicture(file: File) {
      const formData = new FormData();
      formData.append('displayPicture', file);

      const response = await apiClient.patch<UpdateDisplayPictureResponse>(
        `${apiRoutes.user}/profile?dp=true`,
        formData,
      );

      if (this.user) {
        this.user.display_picture = response.data.displayPicture;
      }

      return response.data.displayPicture;
    },

    async removeDisplayPicture() {
      await apiClient.patch(`${apiRoutes.user}/profile?removeDp=true`);

      if (this.user) {
        this.user.display_picture = null;
      }
    },

    clearProfile() {
      this.user = null;
    },
  },
});