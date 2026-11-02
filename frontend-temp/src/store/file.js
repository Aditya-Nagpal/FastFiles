import { defineStore } from 'pinia';
import API from '../api/axios';

export const useFileStore = defineStore('file', {
    state: () => ({
        parentId: null,
        currentKey: '',
        keyStack: [],
        contents: []
    }),

    actions: {
        async fetchContents() {
            try {
                const params = this.parentId ? { parent_id: this.parentId } : {};
                const response = await API.get('/file/list', {
                    params
                });
                console.log('Fetched contents:', response?.data);
                this.contents = response?.data || [];
                return this.contents;
            } catch (error) {
                console.error('Error fetching contents:', error);
                throw error;
            }
        },

        async uploadFile(file) {
            const formData = new FormData();
            formData.append('entityType', 'file');
            formData.append('file', file);
            formData.append('parentId', this.parentId || '');
            try {
                const response = await API.post('/file/upload', formData, {
                    headers: {
                        'Content-Type': 'multipart/form-data'
                    }
                });
                console.log(response.data);
                return response.data;
            } catch (error) {
                console.error('Error uploading file:', error);
                throw error;
            }
        },

        async uploadFolder(folderName) {
            const formData = new FormData();
            formData.append('entityType', 'folder');
            formData.append('parentId', this.parentId || '');
            formData.append('name', folderName);
            try {
                const response = await API.post('/file/upload', formData, {
                    headers: {
                        'Content-Type': 'application/form-data'
                    }
                });
                console.log(response.data);
                return response.data;
            } catch (error) {
                console.error('Error creating folder:', error.AxiosError);
                throw error;
            }
        },

        async enterFolder(folderName, publicId) {
            this.keyStack.push({key: this.currentKey, id: this.parentId});
            this.currentKey += folderName + '/';
            this.parentId = publicId;
            console.log('Current key:', this.currentKey);
            console.log('Parent Id:', this.parentId);
            await this.fetchContents();
        },

        async goBack() {
            if (this.keyStack.length > 0) {
                const { key, id } = this.keyStack.pop();
                this.currentKey = key;
                this.parentId = id;
                console.log('Current key after going back:', this.currentKey);
                console.log('Parent Id after going back:', this.parentId);
                await this.fetchContents();
            }
        },

        async deleteContent(publicId, type) {
            try {
                console.log('Deleting content:', publicId, type, this.currentKey);
                const response = await API.delete(`/file/delete/${publicId}`);
                console.log(response.data);
                return response.data;
            } catch (error) {
                console.error('Error deleting content:', error);
                throw error;
            }
        },

        async downloadFile(publicId) {
            try {
                const response = await API.get(`/file/download/${publicId}`);
                const url = response.data?.downloadURL;
                window.location.href = url;
                return true;
            } catch (error) {
                console.error('Error downloading file:', error);
                throw error;
            }
        }
    }
});