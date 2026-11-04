<script setup lang="ts">
import type { FileItem } from '../../stores/files';
import FileActionsMenu from './FileActionsMenu.vue';

const props = defineProps<{
  items: FileItem[];
  total: number;
  isLoading: boolean;
  error: string;
  currentKey: string;
  canGoBack: boolean;
  downloadingFileId: string | null;
}>();

const emit = defineEmits<{
  (event: 'back'): void;
  (event: 'retry'): void;
  (event: 'open', item: FileItem): void;
  (event: 'download', item: FileItem): void;
  (event: 'delete', item: FileItem): void;
}>();
</script>

<template>
  <div class="file-browser">
    <div class="files-toolbar">
      <button
        v-if="props.canGoBack"
        class="back-button"
        type="button"
        @click="emit('back')"
      >
        ← Back
      </button>

      <span class="current-folder">
        {{ props.currentKey || 'All files' }}
      </span>

      <span class="file-count">
        {{ props.total }} {{ props.total === 1 ? 'item' : 'items' }}
      </span>
    </div>

    <p v-if="props.isLoading" class="files-message" role="status">
      Loading your files…
    </p>

    <div v-else-if="props.error" class="files-message" role="alert">
      <p>{{ props.error }}</p>
      <button class="back-button" type="button" @click="emit('retry')">
        Try again
      </button>
    </div>

    <div v-else-if="props.items.length === 0" class="files-message">
      <div class="empty-icon" aria-hidden="true">▤</div>
      <h2>This folder is empty</h2>
      <p>Files and folders you add will appear here.</p>
    </div>

    <ul v-else class="file-list">
      <li v-for="item in props.items" :key="item.public_id">
        <div class="file-row">
          <button
            v-if="item.type === 'FOLDER'"
            class="file-main folder-main"
            type="button"
            @click="emit('open', item)"
          >
            <span class="file-icon" aria-hidden="true">📁</span>
            <span class="file-name">{{ item.name }}</span>
            <span class="item-kind">Folder</span>
          </button>

          <div v-else class="file-main">
            <span class="file-icon" aria-hidden="true">📄</span>
            <span class="file-name">{{ item.name }}</span>
            <span class="item-kind">File</span>
          </div>

          <FileActionsMenu
            :item="item"
            :downloading="props.downloadingFileId === item.public_id"
            @open="emit('open', $event)"
            @download="emit('download', $event)"
            @delete="emit('delete', $event)"
          />
        </div>
      </li>
    </ul>
  </div>
</template>

<style src="../../assets/styles/FileList.css" scoped></style>