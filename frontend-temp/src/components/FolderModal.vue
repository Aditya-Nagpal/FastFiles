<template>
  <div class="modal fade show d-block" tabindex="-1" v-if="show" style="background-color: rgba(0, 0, 0, 0.5);">
    <div class="modal-dialog modal-dialog-centered">
      <div class="modal-content">

        <div class="modal-header">
          <h5 class="modal-title">Create New Folder</h5>
          <button type="button" class="btn-close" @click="close" />
        </div>

        <div class="modal-body">
          <input
            v-model="folderName"
            type="text"
            class="form-control"
            placeholder="Enter folder name"
          />
        </div>

        <div class="modal-footer">
          <button class="btn btn-secondary" @click="close">Cancel</button>
          <button class="btn btn-primary" :disabled="!folderName.trim()" @click="handleFolderUpload">
            Create
          </button>
        </div>

      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch } from 'vue';
import { useFileStore } from '../store/file';

const props = defineProps({
  show: Boolean
});

const emit = defineEmits(['close']);

const folderName = ref('');
const fileStore = useFileStore();

watch(props, () => {
  if (!props.show) folderName.value = '';
});

const close = () => {
  folderName.value = '';
  emit('close');
};

const handleFolderUpload = async () => {
  try {
    console.log('we are here 2', fileStore.currentKey);
    await fileStore.uploadFolder(folderName.value);
    close();
    await fileStore.fetchContents();
  } catch (error) {
    console.error('Error uploading folder:', error);
  }
};
</script>