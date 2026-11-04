<script setup lang="ts">
import { ref, watch } from 'vue';

const props = defineProps<{
  open: boolean;
  isUploading: boolean;
  error: string;
}>();

const emit = defineEmits<{
  (event: 'close'): void;
  (event: 'upload', file: File): void;
}>();

const fileInput = ref<HTMLInputElement | null>(null);
const selectedFile = ref<File | null>(null);
const isDraggingFile = ref(false);
const validationError = ref('');

watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) {
      selectedFile.value = null;
      validationError.value = '';
      if (fileInput.value) fileInput.value.value = '';
    }
  },
);

function closeModal() {
  if (props.isUploading) return;
  emit('close');
}

function selectFile(file?: File) {
  selectedFile.value = file ?? null;
  validationError.value = '';
}

function handleFileSelection(event: Event) {
  const input = event.target as HTMLInputElement;
  selectFile(input.files?.[0]);
}

function handleFileDrop(event: DragEvent) {
  isDraggingFile.value = false;
  selectFile(event.dataTransfer?.files?.[0]);
}

function submitUpload() {
  if (!selectedFile.value) {
    validationError.value = 'Choose a file first.';
    return;
  }

  emit('upload', selectedFile.value);
}

function formatFileSize(size: number) {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}
</script>

<template>
  <div
    v-if="props.open"
    class="modal-backdrop"
    @click.self="closeModal"
  >
    <section
      class="upload-modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="upload-modal-title"
    >
      <h2 id="upload-modal-title">Upload a file</h2>
      <p class="modal-copy">
        Choose a file from your device or drag it into the area below.
      </p>

      <input
        ref="fileInput"
        class="visually-hidden"
        type="file"
        :disabled="props.isUploading"
        @change="handleFileSelection"
      />

      <div
        class="upload-dropzone"
        :class="{ 'is-dragging': isDraggingFile }"
        @dragover.prevent="isDraggingFile = true"
        @dragleave.prevent="isDraggingFile = false"
        @drop.prevent="handleFileDrop"
      >
        <template v-if="selectedFile">
          <span class="dropzone-icon" aria-hidden="true">📄</span>
          <strong class="selected-file-name">{{ selectedFile.name }}</strong>
          <span class="selected-file-size">
            {{ formatFileSize(selectedFile.size) }}
          </span>
          <button
            class="browse-button"
            type="button"
            :disabled="props.isUploading"
            @click="fileInput?.click()"
          >
            Choose a different file
          </button>
        </template>

        <template v-else>
          <span class="dropzone-icon" aria-hidden="true">↑</span>
          <strong>Drop your file here</strong>
          <span class="selected-file-size">or choose one from your device</span>
          <button
            class="browse-button"
            type="button"
            :disabled="props.isUploading"
            @click="fileInput?.click()"
          >
            Browse files
          </button>
        </template>
      </div>

      <p v-if="validationError || props.error" class="modal-error" role="alert">
        {{ validationError || props.error }}
      </p>

      <div class="modal-actions">
        <button
          class="button button-secondary"
          type="button"
          :disabled="props.isUploading"
          @click="closeModal"
        >
          Cancel
        </button>
        <button
          class="button button-primary"
          type="button"
          :disabled="props.isUploading || !selectedFile"
          @click="submitUpload"
        >
          {{ props.isUploading ? 'Uploading…' : 'Upload file' }}
        </button>
      </div>
    </section>
  </div>
</template>

<style src="../../assets/styles/UploadFileModal.css" scoped></style>