import { isAxiosError } from 'axios';
import { defineStore } from 'pinia';
import { apiClient } from '../api/http';
import { apiRoutes } from '../api/routes';

interface StartResetResponse {
    flowId?: string;
}

const resetFlowStatusCodes = [410, 422, 429];

export const useForgotPasswordStore = defineStore('forgotPassword', {
    state: () => ({
        flowId: localStorage.getItem('flowId'),
        email: localStorage.getItem('resetEmail') ?? '',
    }),

    actions: {
        setFlow(flowId: string, email: string) {
            this.flowId = flowId;
            this.email = email;
            localStorage.setItem('flowId', flowId);
            localStorage.setItem('resetEmail', email);
        },

        resetFlow() {
            this.flowId = null;
            this.email = '';
            localStorage.removeItem('flowId');
            localStorage.removeItem('resetEmail');
        },

        async startForgotPassword(email: string) {
            try {
                const response = await apiClient.post<StartResetResponse>(
                    `${apiRoutes.auth}/forgot-password/start`,
                    { email },
                );

                if (response.status !== 201 || !response.data.flowId) {
                    throw new Error('The password reset response did not include a flow ID.');
                }

                this.setFlow(response.data.flowId, email);
            } catch (error: unknown) {
                if (
                    isAxiosError(error) &&
                    resetFlowStatusCodes.includes(error.response?.status ?? 0)
                ) {
                    this.resetFlow();
                }

                throw error;
            }
        },

        async verifyOtp(otp: string) {
            try {
                return await apiClient.post(`${apiRoutes.auth}/forgot-password/verify`, {
                    flowId: this.flowId,
                    email: this.email,
                    otp,
                });
            } catch (error: unknown) {
                if (
                    isAxiosError(error) &&
                    resetFlowStatusCodes.includes(error.response?.status ?? 0)
                ) {
                    this.resetFlow();
                }

                throw error;
            }
        },

        async resendOtp() {
            try {
                return await apiClient.post(`${apiRoutes.auth}/forgot-password/resend`, {
                    flowId: this.flowId,
                    email: this.email,
                });
            } catch (error: unknown) {
                if (
                    isAxiosError(error) &&
                    resetFlowStatusCodes.includes(error.response?.status ?? 0)
                ) {
                    this.resetFlow();
                }

                throw error;
            }
        },

        async resetPassword(newPassword: string) {
            try {
                const response = await apiClient.patch(
                    `${apiRoutes.auth}/forgot-password/reset`,
                    {
                        flowId: this.flowId,
                        email: this.email,
                        newPassword,
                    },
                );

                this.resetFlow();
                return response.data;
            } catch (error: unknown) {
                if (
                    isAxiosError(error) &&
                    resetFlowStatusCodes.includes(error.response?.status ?? 0)
                ) {
                    this.resetFlow();
                }

                throw error;
            }
        },
    },
});