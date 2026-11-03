import { defineStore } from 'pinia';
import { apiClient } from '../api/http';
import { apiRoutes } from '../api/routes';

export interface FileItem {
    name: string;
    type: 'FILE' | 'FOLDER' | string;
    public_id: string;
    [key: string]: unknown;
}

interface FolderLocation {
    key: string;
    id: string | null;
}

export const useFilesStore = defineStore('files', {
    state: () => ({
        parentId: null as string | null,
        currentKey: '',
        keyStack: [] as FolderLocation[],
        contents: [] as FileItem[],
        isLoading: false,
        errorMessage: '',
    }),

    actions: {
        async fetchContents() {
            this.isLoading = true;
            this.errorMessage = '';

            try {
                const response = await apiClient.get<FileItem[]>(
                    `${apiRoutes.files}/list`,
                    {
                        params: this.parentId ? { parent_id: this.parentId } : {},
                    },
                );

                this.contents = response.data;
                return this.contents;
            } catch (error) {
                this.errorMessage = 'Could not load your files. Please try again.';
                throw error;
            } finally {
                this.isLoading = false;
            }
        },

        async enterFolder(folder: FileItem) {
            if (folder.type !== 'FOLDER') return;

            this.keyStack.push({
                key: this.currentKey,
                id: this.parentId,
            });
            this.currentKey += `${folder.name}/`;
            this.parentId = folder.public_id;

            await this.fetchContents();
        },

        async goBack() {
            const previousLocation = this.keyStack.pop();
            if (!previousLocation) return;

            this.currentKey = previousLocation.key;
            this.parentId = previousLocation.id;

            await this.fetchContents();
        },

        async createFolder(folderName: string) {
            const formData = new FormData();
            formData.append('entityType', 'folder');
            formData.append('parentId', this.parentId ?? '');
            formData.append('name', folderName);

            await apiClient.post(`${apiRoutes.files}/upload`, formData);
            await this.fetchContents();
        },

        async uploadFile(file: File) {
            const formData = new FormData();
            formData.append('entityType', 'file');
            formData.append('file', file);
            formData.append('parentId', this.parentId ?? '');

            await apiClient.post(`${apiRoutes.files}/upload`, formData);
            await this.fetchContents();
        },

        async downloadFile(publicId: string) {
            const response = await apiClient.get<{ downloadURL?: string }>(
                `${apiRoutes.files}/download/${publicId}`,
            );

            const downloadURL = response.data?.downloadURL;

            if (!downloadURL) {
                throw new Error('The download response did not include a download URL.');
            }

            window.location.assign(downloadURL);
        },

        async deleteItem(publicId: string) {
            await apiClient.delete(`${apiRoutes.files}/delete/${publicId}`);
            await this.fetchContents();
        },
    },
});