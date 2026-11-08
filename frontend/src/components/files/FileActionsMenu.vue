<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue';
import type { FileItem } from '../../stores/files';

const props = withDefaults(
  defineProps<{
    item: FileItem;
    downloading?: boolean;
    variant?: 'files' | 'trash';
    restoring?: boolean;
  }>(),
  {
    downloading: false,
    variant: 'files',
    restoring: false,
  },
);

const emit = defineEmits<{
  (event: 'open', item: FileItem): void;
  (event: 'download', item: FileItem): void;
  (event: 'move-to-trash', item: FileItem): void;
  (event: 'restore', item: FileItem): void;
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
  if (props.restoring) return;

  closeMenu();
  emit('restore', props.item);
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
        <button type="button" :disabled="props.restoring" @click="restoreItem">
          {{ props.restoring ? 'Restoring…' : 'Restore' }}
        </button>
      </template>

      <template v-else>
        <button v-if="props.item.type === 'FOLDER'" type="button" @click="openFolder">
          Open folder
        </button>

        <button v-else type="button" :disabled="props.downloading" @click="downloadFile">
          {{ props.downloading ? 'Preparing…' : 'Download' }}
        </button>

        <button class="danger-menu-item" type="button" @click="moveToTrash">
          Move to trash
        </button>
      </template>

      <button type="button" @click="showInfo">
        Information
      </button>
    </div>
  </details>
</template>

<style src="../../assets/styles/FileActionsMenu.css" scoped></style>