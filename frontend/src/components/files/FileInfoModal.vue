<script setup lang="ts">
import { computed } from 'vue';
import type { FileItem } from '../../stores/files';

const props = defineProps<{
    open: boolean;
    item: FileItem | null;
}>();

const emit = defineEmits<{
    (event: 'close'): void;
}>();

const formattedSize = computed(() => {
    const size = props.item?.size;

    if (size == null) return 'Unavailable';
    if (size < 1024) return `${size} B`;
    if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
    if (size < 1024 * 1024 * 1024) {
        return `${(size / (1024 * 1024)).toFixed(1)} MB`;
    }

    return `${(size / (1024 * 1024 * 1024)).toFixed(1)} GB`;
});

function formatDate(value?: string) {
    if (!value) return 'Unavailable';

    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return value;

    return new Intl.DateTimeFormat('en-IN', {
        dateStyle: 'medium',
        timeStyle: 'short',
    }).format(date);
}

const wasModified = computed(() => {
    const { created_at, updated_at } = props.item ?? {};
    return Boolean(updated_at && created_at !== updated_at);
});
</script>

<template>
    <div v-if="open && item" class="info-backdrop" @click.self="emit('close')">
        <section class="info-modal" role="dialog" aria-modal="true" aria-labelledby="file-info-title">
            <header class="info-header">
                <div>
                    <p class="info-eyebrow">ITEM DETAILS</p>
                    <h2 id="file-info-title">Information</h2>
                </div>

                <button class="info-close" type="button" aria-label="Close information" @click="emit('close')">
                    ×
                </button>
            </header>

            <dl class="info-list">
                <div class="info-row">
                    <dt>Name</dt>
                    <dd>{{ item.name }}</dd>
                </div>

                <div class="info-row">
                    <dt>Type</dt>
                    <dd>{{ item.type.toLowerCase() }}</dd>
                </div>

                <div class="info-row">
                    <dt>Content type</dt>
                    <dd>{{ item.content_type || 'Unavailable' }}</dd>
                </div>

                <div class="info-row">
                    <dt>Size</dt>
                    <dd>{{ formattedSize }}</dd>
                </div>

                <div class="info-row">
                    <dt>Created</dt>
                    <dd>{{ formatDate(item.created_at) }}</dd>
                </div>

                <div v-if="wasModified" class="info-row">
                    <dt>Modified</dt>
                    <dd>{{ formatDate(item.updated_at) }}</dd>
                </div>
            </dl>

            <footer class="info-footer">
                <button class="info-done-button" type="button" @click="emit('close')">
                    Done
                </button>
            </footer>
        </section>
    </div>
</template>

<style src="../../assets/styles/FileInfoModal.css" scoped></style>