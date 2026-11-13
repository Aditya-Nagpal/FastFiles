import { defineStore } from 'pinia';
import { apiClient } from '../api/http';
import { apiRoutes } from '../api/routes';

export interface StorageQuota {
    activeBytes: number;
    availableBytes: number;
    quotaBytes: number;
    reservedBytes: number;
    trashBytes: number;
    usagePercentage: number;
    usedBytes: number;
}

export const useStorageQuotaStore = defineStore('storageQuota', {
    state: () => ({
        quota: null as StorageQuota | null,
        isLoading: false,
        errorMessage: '',
    }),

    actions: {
        async fetchQuota() {
            this.isLoading = true;
            this.errorMessage = '';

            try {
                const response = await apiClient.get<StorageQuota>(
                    `${apiRoutes.files}/storage/quota`,
                );

                this.quota = response.data;
                return this.quota;
            } catch (error) {
                this.errorMessage = 'Storage usage could not be loaded.';
                throw error;
            } finally {
                this.isLoading = false;
            }
        },
    },
});