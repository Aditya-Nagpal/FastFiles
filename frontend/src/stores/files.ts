import { defineStore } from 'pinia';
import { apiClient } from '../api/http';
import { apiRoutes } from '../api/routes';

export interface FileItem {
    public_id: string;
    name: string;
    type: string;
    content_type?: string | null;
    extension?: string | null;
    size?: number | null;
    created_at?: string;
    updated_at?: string;
    is_starred?: boolean;
}

interface FileResponse {
    results: FileItem[];
    total: number;
}

interface FolderLocation {
    key: string;
    id: string | null;
}

export type FileSearchMode = 'named' | 'smart';

export const useFilesStore = defineStore('files', {
    state: () => ({
        parentId: null as string | null,
        currentKey: '',
        keyStack: [] as FolderLocation[],

        contents: [] as FileItem[],
        total: 0,
        isLoading: false,
        errorMessage: '',

        searchResults: [] as FileItem[],
        searchTotal: 0,
        isSearching: false,
        searchError: '',
    }),

    actions: {
        async fetchContents() {
            this.isLoading = true;
            this.errorMessage = '';

            try {
                const response = await apiClient.get<FileResponse>(
                    `${apiRoutes.files}/list`,
                    {
                        params: this.parentId ? { parent_id: this.parentId } : {},
                    },
                );

                this.contents = response.data.results;
                this.total = response.data.total;

                return this.contents;
            } catch (error) {
                this.errorMessage = 'Could not load your files. Please try again.';
                throw error;
            } finally {
                this.isLoading = false;
            }
        },

        async searchFiles(
            query: string,
            mode: FileSearchMode,
            limit = 20,
        ) {
            this.isSearching = true;
            this.searchError = '';

            try {
                const response = await apiClient.post<FileResponse>(
                    `${apiRoutes.files}/${apiRoutes.search[mode]}`,
                    {
                        query: query.trim(),
                        limit,
                    },
                );

                this.searchResults = response.data.results;
                this.searchTotal = response.data.total;

                return response.data;
            } catch (error) {
                this.searchError = 'Search failed. Please try again.';
                throw error;
            } finally {
                this.isSearching = false;
            }
        },

        clearSearchResults() {
            this.searchResults = [];
            this.searchTotal = 0;
            this.searchError = '';
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

        async moveToTrash(publicId: string) {
            await apiClient.patch(
                `${apiRoutes.files}/trash/${encodeURIComponent(publicId)}`,
            );

            const wasInContents = this.contents.some(
                (item) => item.public_id === publicId,
            );
            this.contents = this.contents.filter(
                (item) => item.public_id !== publicId,
            );
            if (wasInContents) {
                this.total = Math.max(0, this.total - 1);
            }

            const wasInSearchResults = this.searchResults.some(
                (item) => item.public_id === publicId,
            );
            this.searchResults = this.searchResults.filter(
                (item) => item.public_id !== publicId,
            );
            if (wasInSearchResults) {
                this.searchTotal = Math.max(0, this.searchTotal - 1);
            }
        },
    },
});