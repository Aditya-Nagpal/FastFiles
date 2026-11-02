<template>
  <div class="modal fade show d-block" tabindex="-1" v-if="show" style="background-color: rgba(0,0,0,0.5);">
    <div class="modal-dialog">
      <div class="modal-content">

        <div class="modal-header">
          <h5 class="modal-title text-danger">Confirm Delete</h5>
          <button type="button" class="btn-close" @click="emit('close', false)" />
        </div>

        <div class="modal-body">
          <p>
            Are you sure you want to delete <strong>{{ item.name }}</strong>
            from <strong>{{ fileStore.currentKey || '/' }}</strong>?
          </p>
        </div>

        <div class="modal-footer">
          <button class="btn btn-secondary" @click="emit('close', false)">No</button>
          <button class="btn btn-danger" @click="deleteEntity(props.item)">Yes, Delete</button>
        </div>

      </div>
    </div>
  </div>
</template>

<script setup>
import { useFileStore } from '../store/file.js';

const props = defineProps({
  show: Boolean,
  item: Object,
});

const emit = defineEmits(['close']);

const fileStore = useFileStore();

const deleteEntity =  async (item) => {
  try {
    console.log('Deleting item:', item);
    await fileStore.deleteContent(item.public_id, item.type);
    close();
    await fileStore.fetchContents();
  } catch (error) {
    console.error('Error deleting item:', error);
  }
};

const close = () => {
  emit('close');
};
</script>