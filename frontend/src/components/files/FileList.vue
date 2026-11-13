<script setup lang="ts">
import type { FileItem } from '../../stores/files';
import FileActionsMenu from './FileActionsMenu.vue';
import FileStarButton from './FileStarButton.vue';

const props = withDefaults(
  defineProps<{
    items: FileItem[];
    total: number;
    isLoading: boolean;
    error: string;
    currentKey: string;
    canGoBack: boolean;
    downloadingFileId: string | null;
    updatingStarredIds?: string[];
    emptyTitle?: string;
    emptyDescription?: string;
    variant?: 'files' | 'trash';
    restoringPublicIds?: string[];
    permanentlyDeletingPublicIds?: string[];
  }>(),
  {
    emptyTitle: 'This folder is empty',
    emptyDescription: 'Files and folders you add will appear here.',
    variant: 'files',
  },
);

const emit = defineEmits<{
  (event: 'back'): void;
  (event: 'retry'): void;
  (event: 'open', item: FileItem): void;
  (event: 'download', item: FileItem): void;
  (event: 'move-to-trash', item: FileItem): void;
  (event: 'info', item: FileItem): void;
  (event: 'star', item: FileItem, isStarred: boolean): void;
  (event: 'restore', item: FileItem): void;
  (event: 'permanent-delete', item: FileItem): void;
}>();

function handleStarToggle(item: FileItem, isStarred: boolean) {
  emit('star', item, isStarred);
}
</script>

<template>
  <div class="file-browser">
    <div class="files-toolbar">
      <button v-if="props.canGoBack" class="back-button" type="button" @click="emit('back')">
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
      {{ props.variant === 'trash' ? 'Loading trash…' : 'Loading your files…' }}
    </p>

    <div v-else-if="props.error" class="files-message" role="alert">
      <p>{{ props.error }}</p>
      <button class="back-button" type="button" @click="emit('retry')">
        Try again
      </button>
    </div>

    <div v-else-if="props.items.length === 0" class="files-message">
      <div class="empty-icon" aria-hidden="true">▤</div>
      <h2>{{ props.emptyTitle }}</h2>
      <p>{{ props.emptyDescription }}</p>
    </div>

    <ul v-else class="file-list">
      <li v-for="item in props.items" :key="item.public_id">
        <div class="file-row">
          <button v-if="item.type === 'FOLDER'" class="file-main folder-main" type="button" @click="emit('open', item)">
            <span class="file-icon" aria-hidden="true">📁</span>
            <span class="file-name">{{ item.name }}</span>
            <span class="item-kind">Folder</span>
          </button>

          <div v-else class="file-main">
            <span class="file-icon" aria-hidden="true">📄</span>
            <span class="file-name">{{ item.name }}</span>
            <span class="item-kind">File</span>
          </div>

          <template v-if="props.variant === 'trash'">
            <FileActionsMenu :item="item" variant="trash"
              :restoring="props.restoringPublicIds?.includes(item.public_id) ?? false" :permanently-deleting="props.permanentlyDeletingPublicIds?.includes(item.public_id) ?? false
                " @restore="emit('restore', $event)" @permanent-delete="emit('permanent-delete', $event)"
              @info="emit('info', $event)" />
          </template>

          <template v-else>
            <FileStarButton :item="item" :disabled="props.updatingStarredIds?.includes(item.public_id) ?? false"
              @toggle="handleStarToggle" />

            <FileActionsMenu :item="item" :downloading="props.downloadingFileId === item.public_id"
              @open="emit('open', $event)" @download="emit('download', $event)"
              @move-to-trash="emit('move-to-trash', $event)" @info="emit('info', $event)" />
          </template>
        </div>
      </li>
    </ul>
  </div>
</template>

<style src="../../assets/styles/FileList.css" scoped></style>