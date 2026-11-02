<template>
  <div class="modal fade show d-block" tabindex="-1" v-if="show" style="background-color: rgba(0,0,0,0.5);">
    <div class="modal-dialog">
      <div class="modal-content">

        <div class="modal-header">
          <h5 class="modal-title">Upload File</h5>
          <button type="button" class="btn-close" @click="close" />
        </div>

        <div class="modal-body">
          <input type="file" @change="handleFileChange" class="form-control" />
          <div v-if="selectedFile" class="mt-3">
            <strong>Selected:</strong> {{ selectedFile.name }}
          </div>
        </div>

        <div class="modal-footer">
          <button class="btn btn-secondary" @click="close">Cancel</button>
          <button class="btn btn-primary" :disabled="!selectedFile || uploading" @click="handleFileUpload">
            <span v-if="uploading" class="spinner-border spinner-border-sm me-1"></span>
            Upload
          </button>
        </div>

      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue';
import { useFileStore } from '../store/file';

const props = defineProps({
  show: Boolean
});

const emit = defineEmits(['close']);

const selectedFile = ref(null);
const uploading = ref(false);

const fileStore = useFileStore();

const handleFileChange = (e) => {
  selectedFile.value = e.target.files[0];
};

const close = () => {
  selectedFile.value = null;
  emit('close');
};

const handleFileUpload = async () => {
  if (!selectedFile.value) return;
  uploading.value = true;

  try {
    await fileStore.uploadFile(selectedFile.value);
    close();
    await fileStore.fetchContents();
    selectedFile.value = null;
  } catch (error) {
    console.error('Upload failed:', error);
    selectedFile.value = null;
  } finally {
    uploading.value = false;
  }
};
</script>
