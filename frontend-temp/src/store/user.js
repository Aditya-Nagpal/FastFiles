import { defineStore } from 'pinia';
import API from '../api/axios';

const FETCH_USER_API = `/user/profile`;
const UPDATE_USER_API = `/user/profile`;

export const useUserStore = defineStore('user', {
    state: () => ({
        user: null,
    }),

    actions: {
        async fetchUserProfile() {
            try {
                const response = await API.get(FETCH_USER_API);
                this.user = response.data;
                return this.user;
            } catch (error) {
                console.error('Failed to fetch user:', error);
                throw error;
            }
        },

        async updateUserProfile(payload) {
            try {
                const response = await API.patch(`${UPDATE_USER_API}?dp=false`, payload);
                const updatedUser = {};
                for (let [key, value] of Object.entries(response.data.updatedUser)) {
                    if (["", null, undefined, 0].includes(value)) continue;
                    updatedUser[key] = value;
                }
                this.user = { ...this.user, ...updatedUser };
                console.log('Updated user:', this.user);
                return true;
            } catch (error) {
                console.error('Failed to update user:', error);
                throw error;
            }
        },

        async updateDisplayPicture(payload) {
            try {
                const response = await API.patch(`${UPDATE_USER_API}?dp=true`, payload, {
                    headers: {
                        "Content-Type": "multipart/form-data"
                    }
                });
                this.user.display_picture = response.data.displayPicture;
                return true;
            } catch (error) {
                console.error('Failed to update dp: ', error);
                throw error;
            }
        },

        async removeDisplayPicture() {
            try {
                await API.patch(`${UPDATE_USER_API}?removeDp=true`);
                this.user.display_picture = null;
                return true;
            } catch (error) {
                console.error('Failed to remove dp: ', error);
                throw error;
            }
        }
    }
});