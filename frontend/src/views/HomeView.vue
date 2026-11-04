<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useFilesStore } from '../stores/files';
import type { FileItem, FileSearchMode } from '../stores/files';
import FileSearch from '../components/files/FileSearch.vue';
import FileActionsMenu from '../components/files/FileActionsMenu.vue';
import CreateFolderModal from '../components/files/CreateFolderModal.vue';
import UploadFileModal from '../components/files/UploadFileModal.vue';
import ConfirmDeleteModal from '../components/files/ConfirmDeleteModal.vue';
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

function clearSearch() {
  files.clearSearchResults();
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
    <aside class="sidebar">
      <a class="brand" href="/" aria-label="FastFiles home">
        <span class="brand-mark">F</span>
        <span>FastFiles</span>
      </a>

      <div class="sidebar-section">
        <p class="section-label">Workspace</p>

        <nav class="navigation" aria-label="Main navigation">
          <div class="nav-item is-active" aria-current="page">
            <span class="nav-icon">▤</span>
            <span>Files</span>
          </div>
          <div class="nav-item">
            <span class="nav-icon">♧</span>
            <span>Shared</span>
          </div>
          <div class="nav-item">
            <span class="nav-icon">◷</span>
            <span>Recent</span>
          </div>
          <div class="nav-item">
            <span class="nav-icon">☆</span>
            <span>Starred</span>
          </div>
          <div class="nav-item">
            <span class="nav-icon">▱</span>
            <span>Trash</span>
          </div>
        </nav>
      </div>

      <div class="storage-card">
        <div class="storage-icon">◇</div>
        <p class="storage-title">Storage</p>
        <p class="storage-copy">Usage details will appear when we connect your file service.</p>
      </div>

      <div class="sidebar-bottom">
        <span class="avatar">U</span>
        <div>
          <p class="account-title">Your account</p>
          <p class="account-subtitle">Profile settings</p>
        </div>
      </div>
    </aside>

    <main class="main-area">
      <header class="topbar">
        <div class="breadcrumb">
          <span>Workspace</span>
          <span class="breadcrumb-divider">/</span>
          <strong>Files</strong>
        </div>

        <button class="theme-button" type="button"
          :aria-label="theme === 'light' ? 'Switch to dark mode' : 'Switch to light mode'" @click="toggleTheme">
          <span aria-hidden="true">{{ theme === 'light' ? '☾' : '☀' }}</span>
          <span>{{ theme === 'light' ? 'Dark mode' : 'Light mode' }}</span>
        </button>
      </header>

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
          :error="files.searchError" @search="handleSearch" @clear="clearSearch" @download="downloadFile"
          @open="openFolder" @delete="requestDelete" />

        <p v-if="downloadError" class="upload-error" role="alert">
          {{ downloadError }}
        </p>

        <section class="workspace-card" aria-label="Your files">
          <div class="files-toolbar">
            <button v-if="files.keyStack.length > 0" class="back-button" type="button" @click="goBack">
              ← Back
            </button>

            <span class="current-folder">
              {{ files.currentKey || 'All files' }}
            </span>
          </div>

          <p v-if="files.isLoading" class="files-message" role="status">
            Loading your files…
          </p>

          <div v-else-if="files.errorMessage" class="files-message" role="alert">
            <p>{{ files.errorMessage }}</p>
            <button class="back-button" type="button" @click="loadContents">
              Try again
            </button>
          </div>

          <div v-else-if="files.contents.length === 0" class="files-message">
            <div class="empty-icon" aria-hidden="true">▤</div>
            <h2>This folder is empty</h2>
            <p>Files and folders you add will appear here.</p>
          </div>

          <ul v-else class="file-list">
            <li v-for="item in files.contents" :key="item.public_id">
              <div class="file-row">
                <button v-if="item.type === 'FOLDER'" class="file-main folder-main" type="button"
                  @click="openFolder(item)">
                  <span class="file-icon" aria-hidden="true">📁</span>
                  <span class="file-name">{{ item.name }}</span>
                  <span class="item-kind">Folder</span>
                </button>

                <div v-else class="file-main">
                  <span class="file-icon" aria-hidden="true">📄</span>
                  <span class="file-name">{{ item.name }}</span>
                  <span class="item-kind">File</span>
                </div>

                <FileActionsMenu :item="item" :downloading="downloadingFileId === item.public_id" @open="openFolder"
                  @download="downloadFile" @delete="requestDelete" />
              </div>
            </li>
          </ul>

          <CreateFolderModal :open="isFolderModalOpen" :is-creating="isCreatingFolder" :error="folderError"
            @close="closeNewFolderModal" @create="createFolder" />

          <UploadFileModal :open="isUploadModalOpen" :is-uploading="isUploading" :error="uploadModalError"
            @close="closeUploadModal" @upload="uploadSelectedFile" />

          <ConfirmDeleteModal :open="isDeleteModalOpen" :item="itemToDelete" :is-deleting="isDeleting"
            :error="deleteError" @close="closeDeleteModal" @confirm="confirmDelete" />
        </section>

        <footer class="page-footer">
          <span>FastFiles</span>
          <span>Secure file storage, made simple.</span>
        </footer>
      </section>
    </main>
  </div>
</template>

<style src="../assets/styles/HomeView.css" scoped></style>