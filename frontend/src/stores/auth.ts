import { defineStore } from 'pinia';
import { apiClient } from '../api/http';
import { apiRoutes } from '../api/routes';

interface SignInResponse {
    accessToken: string;
    userId?: string | number;
}

function readStoredValue(key: string): string | null {
    return localStorage.getItem(key);
}

export const useAuthStore = defineStore('auth', {
    state: () => ({
        accessToken: readStoredValue('accessToken'),
        userId: readStoredValue('userId'),
    }),

    getters: {
        isAuthenticated: (state) => Boolean(state.accessToken),
    },

    actions: {
        async signUp(payload: Record<string, unknown>) {
            const response = await apiClient.post(
                `${apiRoutes.auth}/register`,
                payload,
            );

            return response.data;
        },

        async signIn(email: string, password: string) {
            const response = await apiClient.post<SignInResponse>(
                `${apiRoutes.auth}/login`,
                { email, password },
            );

            const { accessToken, userId } = response.data;

            if (!accessToken) {
                throw new Error('The login response did not include an access token.');
            }

            this.setAccessToken(accessToken);
            this.userId = userId == null ? null : String(userId);

            if (this.userId) {
                localStorage.setItem('userId', this.userId);
            } else {
                localStorage.removeItem('userId');
            }

            return response.data;
        },

        async checkAuth() {
            const response = await apiClient.get(
                `${apiRoutes.auth}/protected`,
            );

            return response.data;
        },

        async logout() {
            try {
                await apiClient.get(`${apiRoutes.auth}/logout`);
            } finally {
                this.clearSession();
            }
        },

        setAccessToken(accessToken: string) {
            this.accessToken = accessToken;
            localStorage.setItem('accessToken', accessToken);
        },

        clearSession() {
            this.accessToken = null;
            this.userId = null;
            localStorage.removeItem('accessToken');
            localStorage.removeItem('userId');
        },
    },
});