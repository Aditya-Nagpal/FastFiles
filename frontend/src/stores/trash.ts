import { defineStore } from 'pinia';
import { apiClient } from '../api/http';
import { apiRoutes } from '../api/routes';
import type { FileItem } from './files';

interface TrashResponse {
    results: FileItem[];
    total: number;
}

interface TrashLocation {
    key: string;
    id: string | null;
}

export const useTrashStore = defineStore('trash', {
    state: () => ({
        items: [] as FileItem[],
        total: 0,
        parentId: null as string | null,
        currentKey: '',
        keyStack: [] as TrashLocation[],
        isLoading: false,
        errorMessage: '',
    }),

    actions: {
        async fetchTrash() {
            this.isLoading = true;
            this.errorMessage = '';

            try {
                const response = await apiClient.get<TrashResponse>(
                    `${apiRoutes.files}/trash`,
                    {
                        params: this.parentId ? { parent_id: this.parentId } : {},
                    },
                );

                this.items = response.data.results;
                this.total = response.data.total;
                return response.data;
            } catch (error) {
                this.errorMessage = 'Could not load trash. Please try again.';
                throw error;
            } finally {
                this.isLoading = false;
            }
        },

        async openRoot() {
            this.parentId = null;
            this.currentKey = '';
            this.keyStack = [];
            return this.fetchTrash();
        },

        async enterFolder(folder: FileItem) {
            if (folder.type !== 'FOLDER') return;

            const previousLocation: TrashLocation = {
                key: this.currentKey,
                id: this.parentId,
            };

            this.keyStack.push(previousLocation);
            this.parentId = folder.public_id;
            this.currentKey += `${folder.name}/`;

            try {
                await this.fetchTrash();
            } catch (error) {
                this.keyStack.pop();
                this.parentId = previousLocation.id;
                this.currentKey = previousLocation.key;
                throw error;
            }
        },

        async goBack() {
            const previousLocation = this.keyStack.pop();
            if (!previousLocation) return;

            this.parentId = previousLocation.id;
            this.currentKey = previousLocation.key;
            await this.fetchTrash();
        },

        async restoreItem(publicId: string) {
            await apiClient.patch(
                `${apiRoutes.files}/trash/restore/${encodeURIComponent(publicId)}`,
            );

            const wasInCurrentList = this.items.some(
                (item) => item.public_id === publicId,
            );

            this.items = this.items.filter(
                (item) => item.public_id !== publicId,
            );

            if (wasInCurrentList) {
                this.total = Math.max(0, this.total - 1);
            }
        },

        async permanentlyDeleteItem(publicId: string) {
            await apiClient.delete(
                `${apiRoutes.files}/trash/${encodeURIComponent(publicId)}/permanent`,
            );

            const wasInCurrentList = this.items.some(
                (item) => item.public_id === publicId,
            );

            this.items = this.items.filter(
                (item) => item.public_id !== publicId,
            );

            if (wasInCurrentList) {
                this.total = Math.max(0, this.total - 1);
            }
        },
    },
});