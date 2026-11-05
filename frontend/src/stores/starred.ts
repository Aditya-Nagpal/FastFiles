import { defineStore } from 'pinia';
import { apiClient } from '../api/http';
import { apiRoutes } from '../api/routes';
import { useFilesStore, type FileItem } from './files';

interface StarredResponse {
    results: FileItem[];
    total: number;
}

interface UpdateStarResponse {
    is_starred: boolean;
    message: string;
}

export const useStarredStore = defineStore('starred', {
    state: () => ({
        items: [] as FileItem[],
        total: 0,
        isLoading: false,
        errorMessage: '',
        hasLoaded: false,
        updatingPublicIds: [] as string[],
    }),

    actions: {
        async fetchStarred() {
            this.isLoading = true;
            this.errorMessage = '';

            try {
                const response = await apiClient.get<StarredResponse>(
                    `${apiRoutes.files}/starred`,
                );

                this.items = response.data.results;
                this.total = response.data.total;
                this.hasLoaded = true;

                return response.data;
            } catch (error) {
                this.errorMessage = 'Could not load starred items. Please try again.';
                throw error;
            } finally {
                this.isLoading = false;
            }
        },

        async setStarred(item: FileItem, isStarred: boolean) {
            const publicId = item.public_id;

            if (this.updatingPublicIds.includes(publicId)) return;

            this.updatingPublicIds.push(publicId);

            try {
                const response = await apiClient.patch<UpdateStarResponse>(
                    `${apiRoutes.files}/starred/${encodeURIComponent(publicId)}`,
                    { is_starred: isStarred },
                );

                const updatedValue = response.data.is_starred;
                const filesStore = useFilesStore();

                filesStore.contents = filesStore.contents.map((entry) =>
                    entry.public_id === publicId
                        ? { ...entry, is_starred: updatedValue }
                        : entry,
                );

                filesStore.searchResults = filesStore.searchResults.map((entry) =>
                    entry.public_id === publicId
                        ? { ...entry, is_starred: updatedValue }
                        : entry,
                );

                const alreadyInStarred = this.items.some(
                    (entry) => entry.public_id === publicId,
                );

                if (updatedValue && !alreadyInStarred) {
                    this.items = [{ ...item, is_starred: true }, ...this.items];

                    if (this.hasLoaded) {
                        this.total += 1;
                    }
                } else if (!updatedValue && alreadyInStarred) {
                    this.items = this.items.filter(
                        (entry) => entry.public_id !== publicId,
                    );

                    if (this.hasLoaded) {
                        this.total = Math.max(0, this.total - 1);
                    }
                }

                return response.data;
            } finally {
                this.updatingPublicIds = this.updatingPublicIds.filter(
                    (id) => id !== publicId,
                );
            }
        },
    },
});