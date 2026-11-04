<script setup lang="ts">
import { ref } from 'vue';

const props = defineProps<{
  imageUrl?: string | null;
  initial: string;
  isUploading: boolean;
  isRemoving: boolean;
  error: string;
}>();

const emit = defineEmits<{
  (event: 'upload', file: File): void;
  (event: 'remove'): void;
}>();

const fileInput = ref<HTMLInputElement | null>(null);
const menuElement = ref<HTMLDetailsElement | null>(null);
const localError = ref('');

function closeMenu() {
  if (menuElement.value) {
    menuElement.value.open = false;
  }
}

function openFilePicker() {
  closeMenu();
  fileInput.value?.click();
}

function handleFileChange(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = '';

  if (!file) return;

  if (!file.type.startsWith('image/')) {
    localError.value = 'Choose an image file.';
    return;
  }

  localError.value = '';
  emit('upload', file);
}

function removePhoto() {
  closeMenu();
  emit('remove');
}
</script>

<template>
  <div class="profile-photo-editor">
    <input
      ref="fileInput"
      class="profile-photo-input"
      type="file"
      accept="image/*"
      @change="handleFileChange"
    />

    <details ref="menuElement" class="profile-photo-menu">
      <summary
        class="profile-photo-trigger"
        aria-label="Profile photo actions"
        title="Profile photo actions"
        :aria-disabled="props.isUploading || props.isRemoving"
      >
        <img
          v-if="props.imageUrl"
          class="profile-photo-avatar"
          :src="props.imageUrl"
          alt="Profile picture"
        />
        <span v-else class="profile-photo-avatar profile-photo-fallback" aria-hidden="true">
          {{ props.initial || 'U' }}
        </span>

        <span class="profile-photo-camera" aria-hidden="true">⌕</span>
      </summary>

      <div class="profile-photo-menu-panel">
        <button
          type="button"
          role="menuitem"
          :disabled="props.isUploading || props.isRemoving"
          @click="openFilePicker"
        >
          {{ props.isUploading ? 'Uploading…' : props.imageUrl ? 'Change photo' : 'Upload photo' }}
        </button>

        <button
          v-if="props.imageUrl"
          class="profile-photo-remove"
          type="button"
          role="menuitem"
          :disabled="props.isUploading || props.isRemoving"
          @click="removePhoto"
        >
          {{ props.isRemoving ? 'Removing…' : 'Remove photo' }}
        </button>
      </div>
    </details>

    <p v-if="localError || props.error" class="profile-photo-error" role="alert">
      {{ localError || props.error }}
    </p>
  </div>
</template>

<style src="../../assets/styles/ProfilePhotoEditor.css" scoped></style>