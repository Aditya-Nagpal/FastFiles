<template>
  <div class="modal fade show d-block" tabindex="-1" v-if="show" style="background-color: rgba(0,0,0,0.5);">
    <div class="modal-dialog">
      <div class="modal-content">

        <div class="modal-header">
          <h5 class="modal-title">File Information</h5>
          <button type="button" class="btn-close" @click="emit('close')" />
        </div>

        <div class="modal-body">
          <ul class="list-group">
            <li class="list-group-item"><strong>Name:</strong> {{ item.name }}</li>
            <li class="list-group-item"><strong>Type:</strong> {{ item.type?.toLowerCase() }}</li>
            <li class="list-group-item"><strong>Content Type:</strong> {{ item.content_type }}</li>
            <li class="list-group-item"><strong>Size:</strong> {{ item.size }} bytes</li>
            <li class="list-group-item"><strong>Created At:</strong> {{ item.created_at }}</li>
            <li class="list-group-item"><strong>Modified At:</strong> {{ item.updated_at === item.created_at ? '' : item.updated_at }}</li>
          </ul>
        </div>

        <div class="modal-footer">
          <button class="btn btn-secondary" @click="emit('close')">Close</button>
        </div>

      </div>
    </div>
  </div>
</template>

<script setup>
const props = defineProps({
  show: Boolean,
  item: Object
})

const emit = defineEmits(['close'])
</script>