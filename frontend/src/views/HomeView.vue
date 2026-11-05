<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useFilesStore } from '../stores/files';
import type { FileItem, FileSearchMode } from '../stores/files';
import FileSearch from '../components/files/FileSearch.vue';
import CreateFolderModal from '../components/files/CreateFolderModal.vue';
import UploadFileModal from '../components/files/UploadFileModal.vue';
import ConfirmDeleteModal from '../components/files/ConfirmDeleteModal.vue';
import FileList from '../components/files/FileList.vue';
import AppSidebar from '../components/layout/AppSidebar.vue';
import AppTopbar from '../components/layout/AppTopbar.vue';
import AppFooter from '../components/layout/AppFooter.vue';
import FileInfoModal from '../components/files/FileInfoModal.vue';
import { useStorageQuotaStore } from '../stores/storageQuota';
import { useStarredStore } from '../stores/starred';
import { isAxiosError } from 'axios';

type Theme = 'light' | 'dark';

const theme = ref<Theme>('light');
const files = useFilesStore();

const isFolderModalOpen = ref(false);
const isCreatingFolder = ref(false);
const folderError = ref('');

const isUploadModalOpen = ref(false);
const isUploading = ref(false);
const uploadModalError = ref('');

const downloadingFileId = ref<string | null>(null);
const downloadError = ref('');

const isDeleteModalOpen = ref(false);
const itemToDelete = ref<FileItem | null>(null);
const isDeleting = ref(false);
const deleteError = ref('');

const itemForInfo = ref<FileItem | null>(null);

const storageQuota = useStorageQuotaStore();

const starred = useStarredStore();
const starError = ref('');

function applyTheme(value: Theme) {
  theme.value = value;
  document.documentElement.dataset.theme = value;
  localStorage.setItem('fastfiles-theme', value);
}

function toggleTheme() {
  applyTheme(theme.value === 'light' ? 'dark' : 'light');
}

function openNewFolderModal() {
  folderError.value = '';
  isFolderModalOpen.value = true;
}

function closeNewFolderModal() {
  if (isCreatingFolder.value) return;
  isFolderModalOpen.value = false;
}

async function createFolder(folderName: string) {
  isCreatingFolder.value = true;
  folderError.value = '';

  try {
    await files.createFolder(folderName);
    isFolderModalOpen.value = false;
  } catch (error: unknown) {
    console.error('Folder creation flow failed:', error);

    if (isAxiosError<{ message?: string; detail?: string }>(error)) {
      console.error('Request URL:', error.config?.baseURL, error.config?.url);
      console.error('HTTP status:', error.response?.status);
      console.error('API response:', error.response?.data);

      folderError.value =
        error.response?.data?.message ??
        error.response?.data?.detail ??
        (error.response
          ? `Request failed (${error.response.status}).`
          : 'No response from the API. Check the gateway, network, or CORS settings.');
    } else {
      folderError.value = 'Could not create the folder. Check the browser console.';
    }
  } finally {
    isCreatingFolder.value = false;
  }
}

function openUploadModal() {
  uploadModalError.value = '';
  isUploadModalOpen.value = true;
}

function closeUploadModal() {
  if (isUploading.value) return;

  isUploadModalOpen.value = false;
  uploadModalError.value = '';
}

async function uploadSelectedFile(file: File) {
  isUploading.value = true;
  uploadModalError.value = '';

  try {
    await files.uploadFile(file);
    refreshStorageQuota();
    isUploadModalOpen.value = false;
  } catch (error) {
    console.error('File upload failed:', error);
    uploadModalError.value = 'Could not upload the file. Please try again.';
  } finally {
    isUploading.value = false;
  }
}

async function downloadFile(item: FileItem) {
  downloadingFileId.value = item.public_id;
  downloadError.value = '';

  try {
    await files.downloadFile(item.public_id);
  } catch (error) {
    console.error('File download failed:', error);
    downloadError.value = 'Could not download the file. Please try again.';
  } finally {
    downloadingFileId.value = null;
  }
}

function requestDelete(item: FileItem, event?: MouseEvent) {
  (event?.currentTarget as HTMLElement | undefined)
    ?.closest('details')
    ?.removeAttribute('open');

  itemToDelete.value = item;
  deleteError.value = '';
  isDeleteModalOpen.value = true;
}

function closeDeleteModal() {
  if (isDeleting.value) return;

  isDeleteModalOpen.value = false;
  itemToDelete.value = null;
  deleteError.value = '';
}

async function confirmDelete() {
  if (!itemToDelete.value) return;

  isDeleting.value = true;
  deleteError.value = '';

  try {
    await files.deleteItem(itemToDelete.value.public_id);
    refreshStorageQuota();
    isDeleteModalOpen.value = false;
    itemToDelete.value = null;
  } catch (error) {
    console.error('Delete failed:', error);
    deleteError.value = 'Could not delete this item. Please try again.';
  } finally {
    isDeleting.value = false;
  }
}

async function handleSearch(payload: {
  query: string;
  mode: FileSearchMode;
}) {
  try {
    await files.searchFiles(payload.query, payload.mode);
  } catch {
    // The store exposes the search error for the component to display.
  }
}

async function updateStar(item: FileItem, isStarred: boolean) {
  starError.value = '';

  try {
    await starred.setStarred(item, isStarred);
  } catch (error) {
    console.error('Could not update starred state:', error);
    starError.value = `Could not ${isStarred ? 'star' : 'unstar'} this item. Please try again.`;
  }
}

function refreshStorageQuota() {
  void storageQuota.fetchQuota().catch((error) => {
    console.error('Could not refresh storage quota:', error);
  });
}

function clearSearch() {
  files.clearSearchResults();
}

function showFileInfo(item: FileItem) {
  itemForInfo.value = item;
}

function closeFileInfo() {
  itemForInfo.value = null;
}

async function loadContents() {
  try {
    await files.fetchContents();
  } catch {
    // The store exposes the error message for the page to display.
  }
}

async function openFolder(item: FileItem) {
  if (item.type !== 'FOLDER') return;

  try {
    await files.enterFolder(item);
  } catch {
    // The store exposes the error message for the page to display.
  }
}

async function goBack() {
  try {
    await files.goBack();
  } catch {
    // The store exposes the error message for the page to display.
  }
}

onMounted(() => {
  const savedTheme = localStorage.getItem('fastfiles-theme');
  applyTheme(savedTheme === 'dark' ? 'dark' : 'light');
  void loadContents();
});
</script>

<template>
  <div class="app-shell">
    <AppSidebar />

    <main class="main-area">
      <AppTopbar :theme="theme" @toggle-theme="toggleTheme" />

      <section class="content">
        <div class="page-heading">
          <div>
            <p class="eyebrow">YOUR WORKSPACE</p>
            <h1>Your files</h1>
            <p class="page-description">
              A calmer place to keep everything organized.
            </p>
          </div>

          <div class="heading-actions">
            <button class="button button-secondary" type="button" @click="openNewFolderModal">
              <span aria-hidden="true">＋</span>
              New folder
            </button>
            <button class="button button-primary" type="button" @click="openUploadModal">
              <span aria-hidden="true">↑</span>
              Upload
            </button>
          </div>
        </div>

        <FileSearch :results="files.searchResults" :total="files.searchTotal" :is-searching="files.isSearching"
          :error="files.searchError" :updating-starred-ids="starred.updatingPublicIds" @search="handleSearch"
          @clear="clearSearch" @download="downloadFile" @open="openFolder" @delete="requestDelete" @star="updateStar" />

        <p v-if="downloadError" class="upload-error" role="alert">
          {{ downloadError }}
        </p>

        <p v-if="starError" class="upload-error" role="alert">
          {{ starError }}
        </p>

        <section class="workspace-card" aria-label="Your files">
          <FileList :items="files.contents" :total="files.total" :is-loading="files.isLoading"
            :error="files.errorMessage" :current-key="files.currentKey" :can-go-back="files.keyStack.length > 0"
            :downloading-file-id="downloadingFileId" :updating-starred-ids="starred.updatingPublicIds" @back="goBack"
            @retry="loadContents" @open="openFolder" @download="downloadFile" @delete="requestDelete"
            @info="showFileInfo" @star="updateStar" />

          <CreateFolderModal :open="isFolderModalOpen" :is-creating="isCreatingFolder" :error="folderError"
            @close="closeNewFolderModal" @create="createFolder" />

          <UploadFileModal :open="isUploadModalOpen" :is-uploading="isUploading" :error="uploadModalError"
            @close="closeUploadModal" @upload="uploadSelectedFile" />

          <ConfirmDeleteModal :open="isDeleteModalOpen" :item="itemToDelete" :is-deleting="isDeleting"
            :error="deleteError" @close="closeDeleteModal" @confirm="confirmDelete" />

          <FileInfoModal :open="itemForInfo !== null" :item="itemForInfo" @close="closeFileInfo" />
        </section>

        <AppFooter />
      </section>
    </main>
  </div>
</template>

<style src="../assets/styles/HomeView.css" scoped></style>