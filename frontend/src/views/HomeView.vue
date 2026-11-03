<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useFilesStore } from '../stores/files';
import type { FileItem } from '../stores/files';
import { isAxiosError } from 'axios';

type Theme = 'light' | 'dark';

const theme = ref<Theme>('light');
const files = useFilesStore();

const isFolderModalOpen = ref(false);
const newFolderName = ref('');
const isCreatingFolder = ref(false);
const folderError = ref('');

const fileInput = ref<HTMLInputElement | null>(null);
const isUploadModalOpen = ref(false);
const isDraggingFile = ref(false);
const selectedFile = ref<File | null>(null);
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
  newFolderName.value = '';
  folderError.value = '';
  isFolderModalOpen.value = true;
}

function closeNewFolderModal() {
  if (isCreatingFolder.value) return;
  isFolderModalOpen.value = false;
}

async function createFolder() {
  const folderName = newFolderName.value.trim();

  if (!folderName) {
    folderError.value = 'Enter a folder name.';
    return;
  }

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
  selectedFile.value = null;
  uploadModalError.value = '';
  isUploadModalOpen.value = true;
}

function closeUploadModal(force = false) {
  if (isUploading.value && !force) return;

  isUploadModalOpen.value = false;
  selectedFile.value = null;
  uploadModalError.value = '';

  if (fileInput.value) {
    fileInput.value.value = '';
  }
}

function handleFileSelection(event: Event) {
  const input = event.target as HTMLInputElement;
  selectedFile.value = input.files?.[0] ?? null;
  uploadModalError.value = '';
}

function handleFileDrop(event: DragEvent) {
  isDraggingFile.value = false;
  selectedFile.value = event.dataTransfer?.files?.[0] ?? null;
  uploadModalError.value = '';
}

async function uploadSelectedFile() {
  if (!selectedFile.value) {
    uploadModalError.value = 'Choose a file first.';
    return;
  }

  isUploading.value = true;
  uploadModalError.value = '';

  try {
    await files.uploadFile(selectedFile.value);
    closeUploadModal(true);
  } catch (error) {
    console.error('File upload failed:', error);
    uploadModalError.value = 'Could not upload the file. Please try again.';
  } finally {
    isUploading.value = false;
  }
}

function formatFileSize(size: number) {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
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

function requestDelete(item: FileItem, event: MouseEvent) {
  (event.currentTarget as HTMLElement)
    .closest('details')
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

            <input ref="fileInput" class="visually-hidden" type="file" @change="handleFileSelection" />
          </div>
        </div>

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

                <details class="file-actions">
                  <summary class="file-actions-trigger" :aria-label="`Actions for ${item.name}`" title="More actions">
                    ⋯
                  </summary>

                  <div class="file-actions-menu">
                    <button v-if="item.type === 'FOLDER'" type="button" @click="openFolder(item)">
                      Open folder
                    </button>

                    <button v-else type="button" :disabled="downloadingFileId === item.public_id"
                      @click="downloadFile(item)">
                      {{ downloadingFileId === item.public_id ? 'Preparing…' : 'Download' }}
                    </button>

                    <button class="danger-menu-item" type="button" @click="requestDelete(item, $event)">
                      Delete
                    </button>
                  </div>
                </details>
              </div>
            </li>
          </ul>

          <div v-if="isFolderModalOpen" class="modal-backdrop" @click.self="closeNewFolderModal">
            <section class="folder-modal" role="dialog" aria-modal="true" aria-labelledby="folder-modal-title">
              <h2 id="folder-modal-title">Create a folder</h2>
              <p class="folder-modal-copy">Choose a name for your new folder.</p>

              <form @submit.prevent="createFolder">
                <label class="folder-label" for="new-folder-name">Folder name</label>
                <input id="new-folder-name" v-model="newFolderName" class="folder-input" type="text" maxlength="120"
                  autocomplete="off" placeholder="e.g. Project files" :disabled="isCreatingFolder" autofocus />

                <p v-if="folderError" class="folder-error" role="alert">
                  {{ folderError }}
                </p>

                <div class="folder-modal-actions">
                  <button class="button button-secondary" type="button" :disabled="isCreatingFolder"
                    @click="closeNewFolderModal">
                    Cancel
                  </button>
                  <button class="button button-primary" type="submit" :disabled="isCreatingFolder">
                    {{ isCreatingFolder ? 'Creating…' : 'Create folder' }}
                  </button>
                </div>
              </form>
            </section>
          </div>

          <div v-if="isUploadModalOpen" class="modal-backdrop" @click.self="() => closeUploadModal()">
            <section class="folder-modal upload-modal" role="dialog" aria-modal="true"
              aria-labelledby="upload-modal-title">
              <h2 id="upload-modal-title">Upload a file</h2>
              <p class="folder-modal-copy">
                Choose a file from your device or drag it into the area below.
              </p>

              <input ref="fileInput" class="visually-hidden" type="file" :disabled="isUploading"
                @change="handleFileSelection" />

              <div class="upload-dropzone" :class="{ 'is-dragging': isDraggingFile }"
                @dragover.prevent="isDraggingFile = true" @dragleave.prevent="isDraggingFile = false"
                @drop.prevent="handleFileDrop">
                <template v-if="selectedFile">
                  <span class="upload-dropzone-icon" aria-hidden="true">📄</span>
                  <strong class="selected-file-name">{{ selectedFile.name }}</strong>
                  <span class="selected-file-size">
                    {{ formatFileSize(selectedFile.size) }}
                  </span>
                  <button class="browse-button" type="button" :disabled="isUploading" @click="fileInput?.click()">
                    Choose a different file
                  </button>
                </template>

                <template v-else>
                  <span class="upload-dropzone-icon" aria-hidden="true">↑</span>
                  <strong>Drop your file here</strong>
                  <span class="selected-file-size">or choose one from your device</span>
                  <button class="browse-button" type="button" :disabled="isUploading" @click="fileInput?.click()">
                    Browse files
                  </button>
                </template>
              </div>

              <p v-if="uploadModalError" class="folder-error" role="alert">
                {{ uploadModalError }}
              </p>

              <div class="folder-modal-actions">
                <button class="button button-secondary" type="button" :disabled="isUploading"
                  @click="() => closeUploadModal()">
                  Cancel
                </button>
                <button class="button button-primary" type="button" :disabled="isUploading || !selectedFile"
                  @click="uploadSelectedFile">
                  {{ isUploading ? 'Uploading…' : 'Upload file' }}
                </button>
              </div>
            </section>
          </div>

          <div v-if="isDeleteModalOpen && itemToDelete" class="modal-backdrop" @click.self="closeDeleteModal">
            <section class="folder-modal delete-modal" role="dialog" aria-modal="true"
              aria-labelledby="delete-modal-title">
              <div class="delete-warning-icon" aria-hidden="true">!</div>

              <h2 id="delete-modal-title">Delete this item?</h2>
              <p class="folder-modal-copy">
                Are you sure you want to delete
                <strong>{{ itemToDelete.name }}</strong>?
              </p>

              <p v-if="deleteError" class="folder-error" role="alert">
                {{ deleteError }}
              </p>

              <div class="folder-modal-actions">
                <button class="button button-secondary" type="button" :disabled="isDeleting" @click="closeDeleteModal">
                  Cancel
                </button>
                <button class="button button-danger" type="button" :disabled="isDeleting" @click="confirmDelete">
                  {{ isDeleting ? 'Deleting…' : 'Delete' }}
                </button>
              </div>
            </section>
          </div>
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