<script setup lang="ts">
import type { FileItem } from '../../stores/files';

const props = defineProps<{
  open: boolean;
  item: FileItem | null;
  isDeleting: boolean;
  error: string;
}>();

const emit = defineEmits<{
  (event: 'close'): void;
  (event: 'confirm'): void;
}>();

function closeModal() {
  if (props.isDeleting) return;
  emit('close');
}

function confirmDelete() {
  if (!props.item || props.isDeleting) return;
  emit('confirm');
}
</script>

<template>
  <div
    v-if="props.open && props.item"
    class="modal-backdrop"
    @click.self="closeModal"
  >
    <section
      class="delete-modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="delete-modal-title"
    >
      <div class="delete-warning-icon" aria-hidden="true">!</div>

      <h2 id="delete-modal-title">Delete this item?</h2>
      <p class="modal-copy">
        Are you sure you want to delete
        <strong>{{ props.item.name }}</strong>?
      </p>

      <p v-if="props.error" class="modal-error" role="alert">
        {{ props.error }}
      </p>

      <div class="modal-actions">
        <button
          class="button button-secondary"
          type="button"
          :disabled="props.isDeleting"
          @click="closeModal"
        >
          Cancel
        </button>
        <button
          class="button button-danger"
          type="button"
          :disabled="props.isDeleting"
          @click="confirmDelete"
        >
          {{ props.isDeleting ? 'Deleting…' : 'Delete' }}
        </button>
      </div>
    </section>
  </div>
</template>

<style src="../../assets/styles/ConfirmDeleteModal.css" scoped></style>