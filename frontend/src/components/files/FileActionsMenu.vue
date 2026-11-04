<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue';
import type { FileItem } from '../../stores/files';

const props = withDefaults(
  defineProps<{
    item: FileItem;
    downloading?: boolean;
  }>(),
  {
    downloading: false,
  },
);

const emit = defineEmits<{
  (event: 'open', item: FileItem): void;
  (event: 'download', item: FileItem): void;
  (event: 'delete', item: FileItem): void;
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

  if (
    menu?.open &&
    target instanceof Node &&
    !menu.contains(target)
  ) {
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

function deleteItem() {
  closeMenu();
  emit('delete', props.item);
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
    <summary
      class="file-actions-trigger"
      :aria-label="`Actions for ${props.item.name}`"
      title="More actions"
    >
      ⋯
    </summary>

    <div class="file-actions-menu">
      <button
        v-if="props.item.type === 'FOLDER'"
        type="button"
        @click="openFolder"
      >
        Open folder
      </button>

      <button
        v-else
        type="button"
        :disabled="props.downloading"
        @click="downloadFile"
      >
        {{ props.downloading ? 'Preparing…' : 'Download' }}
      </button>

      <button class="danger-menu-item" type="button" @click="deleteItem">
        Delete
      </button>
    </div>
  </details>
</template>

<style src="../../assets/styles/FileActionsMenu.css" scoped></style>