<script setup lang="ts">
import { ref, watch } from 'vue';

const props = defineProps<{
  open: boolean;
  isCreating: boolean;
  error: string;
}>();

const emit = defineEmits<{
  (event: 'close'): void;
  (event: 'create', folderName: string): void;
}>();

const folderName = ref('');
const validationError = ref('');

watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) {
      folderName.value = '';
      validationError.value = '';
    }
  },
);

function closeModal() {
  if (props.isCreating) return;
  emit('close');
}

function submitForm() {
  const trimmedName = folderName.value.trim();

  if (!trimmedName) {
    validationError.value = 'Enter a folder name.';
    return;
  }

  validationError.value = '';
  emit('create', trimmedName);
}
</script>

<template>
  <div
    v-if="props.open"
    class="modal-backdrop"
    @click.self="closeModal"
  >
    <section
      class="folder-modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="folder-modal-title"
    >
      <h2 id="folder-modal-title">Create a folder</h2>
      <p class="folder-modal-copy">Choose a name for your new folder.</p>

      <form @submit.prevent="submitForm">
        <label class="folder-label" for="new-folder-name">
          Folder name
        </label>
        <input
          id="new-folder-name"
          v-model="folderName"
          class="folder-input"
          type="text"
          maxlength="120"
          autocomplete="off"
          placeholder="e.g. Project files"
          :disabled="props.isCreating"
          autofocus
          @input="validationError = ''"
        />

        <p v-if="validationError || props.error" class="folder-error" role="alert">
          {{ validationError || props.error }}
        </p>

        <div class="folder-modal-actions">
          <button
            class="button button-secondary"
            type="button"
            :disabled="props.isCreating"
            @click="closeModal"
          >
            Cancel
          </button>
          <button
            class="button button-primary"
            type="submit"
            :disabled="props.isCreating"
          >
            {{ props.isCreating ? 'Creating…' : 'Create folder' }}
          </button>
        </div>
      </form>
    </section>
  </div>
</template>

<style src="../../assets/styles/CreateFolderModal.css" scoped></style>