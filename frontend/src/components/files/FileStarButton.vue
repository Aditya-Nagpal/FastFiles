<script setup lang="ts">
import type { FileItem } from '../../stores/files';

const props = defineProps<{
    item: FileItem;
    disabled?: boolean;
}>();

const emit = defineEmits<{
    (event: 'toggle', item: FileItem, isStarred: boolean): void;
}>();
</script>

<template>
    <button class="file-star-button" :class="{ 'is-starred': props.item.is_starred }" type="button"
        :disabled="props.disabled" :aria-label="props.item.is_starred ? 'Remove from starred' : 'Add to starred'"
        :aria-pressed="Boolean(props.item.is_starred)"
        :title="props.item.is_starred ? 'Remove from starred' : 'Add to starred'"
        @click.stop="emit('toggle', props.item, !props.item.is_starred)">
        {{ props.item.is_starred ? '★' : '☆' }}
    </button>
</template>

<style src="../../assets/styles/FileStarButton.css" scoped></style>