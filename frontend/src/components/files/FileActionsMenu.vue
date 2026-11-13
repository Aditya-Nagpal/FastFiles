<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue';
import type { FileItem } from '../../stores/files';

const props = withDefaults(
  defineProps<{
    item: FileItem;
    downloading?: boolean;
    variant?: 'files' | 'trash';
    restoring?: boolean;
    permanentlyDeleting?: boolean;
  }>(),
  {
    downloading: false,
    variant: 'files',
    restoring: false,
    permanentlyDeleting: false,
  },
);

const emit = defineEmits<{
  (event: 'open', item: FileItem): void;
  (event: 'download', item: FileItem): void;
  (event: 'move-to-trash', item: FileItem): void;
  (event: 'restore', item: FileItem): void;
  (event: 'permanent-delete', item: FileItem): void;
  (event: 'info', item: FileItem): void;
}>();

const menuElement = ref<HTMLDetailsElement | null>(null);

function closeMenu() {
  if (menuElement.value) {
    menuElement.value.open = false;
  }
}

function handleOutsidePointerDown(event: PointerEvent) {
  const menu = menuElement.value;
  const target = event.target;

  if (menu?.open && target instanceof Node && !menu.contains(target)) {
    closeMenu();
  }
}

function openFolder() {
  closeMenu();
  emit('open', props.item);
}

function downloadFile() {
  closeMenu();
  emit('download', props.item);
}

function moveToTrash() {
  closeMenu();
  emit('move-to-trash', props.item);
}

function restoreItem() {
  if (props.restoring || props.permanentlyDeleting) return;

  closeMenu();
  emit('restore', props.item);
}

function permanentlyDeleteItem() {
  if (props.restoring || props.permanentlyDeleting) return;

  closeMenu();
  emit('permanent-delete', props.item);
}

function showInfo() {
  closeMenu();
  emit('info', props.item);
}

onMounted(() => {
  document.addEventListener('pointerdown', handleOutsidePointerDown);
});

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', handleOutsidePointerDown);
});
</script>

<template>
  <details ref="menuElement" class="file-actions">
    <summary class="file-actions-trigger" :aria-label="`Actions for ${props.item.name}`" title="More actions">
      ⋯
    </summary>

    <div class="file-actions-menu">
      <template v-if="props.variant === 'trash'">
        <button type="button" :disabled="props.restoring || props.permanentlyDeleting" @click="restoreItem">
          <span>{{ props.restoring ? 'Restoring…' : 'Restore' }}</span>
          <svg class="file-action-icon" viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M4 10a8 8 0 1 1 2.3 5.7" />
            <path d="M4 4v6h6" />
          </svg>
        </button>

        <button class="danger-menu-item" type="button" :disabled="props.restoring || props.permanentlyDeleting"
          @click="permanentlyDeleteItem">
          <span>{{ props.permanentlyDeleting ? 'Deleting…' : 'Permanently delete' }}</span>
          <svg class="file-action-icon" viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M4 7h16" />
            <path d="M9 7V4h6v3" />
            <path d="m6 7 1 13h10l1-13" />
            <path d="M10 11v5M14 11v5" />
          </svg>
        </button>
      </template>

      <template v-else>
        <button v-if="props.item.type === 'FOLDER'" type="button" @click="openFolder">
          <span>Open folder</span>
          <svg class="file-action-icon" viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M3.5 6.5h6l2 2H20a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1v-10a1 1 0 0 1 .5-1Z" />
            <path d="M3.5 10h17" />
          </svg>
        </button>

        <button v-else type="button" :disabled="props.downloading" @click="downloadFile">
          <span>{{ props.downloading ? 'Preparing…' : 'Download' }}</span>
          <svg class="file-action-icon" viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M12 3v11" />
            <path d="m8 10 4 4 4-4" />
            <path d="M5 17v3h14v-3" />
          </svg>
        </button>

        <button class="danger-menu-item" type="button" @click="moveToTrash">
          <span>Move to trash</span>
          <svg class="file-action-icon" viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M4 7h16" />
            <path d="M9 7V4h6v3" />
            <path d="m6 7 1 13h10l1-13" />
            <path d="M10 11v5M14 11v5" />
          </svg>
        </button>
      </template>

      <button type="button" @click="showInfo">
        <span>Information</span>
        <svg class="file-action-icon" viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <circle cx="12" cy="12" r="9" />
          <path d="M12 11v5" />
          <path d="M12 8h.01" />
        </svg>
      </button>
    </div>
  </details>
</template>

<style src="../../assets/styles/FileActionsMenu.css" scoped></style>