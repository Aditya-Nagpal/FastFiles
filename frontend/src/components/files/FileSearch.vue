<script setup lang="ts">
import { ref } from 'vue';
import type { FileItem, FileSearchMode } from '../../stores/files';
import FileActionsMenu from './FileActionsMenu.vue';
import FileStarButton from './FileStarButton.vue';

interface SearchPayload {
    query: string;
    mode: FileSearchMode;
}

const props = defineProps<{
    results: FileItem[];
    total: number;
    isSearching: boolean;
    error: string;
    updatingStarredIds?: string[];
}>();

const emit = defineEmits<{
    (event: 'search', payload: SearchPayload): void;
    (event: 'clear'): void;
    (event: 'download', item: FileItem): void;
    (event: 'open', item: FileItem): void;
    (event: 'move-to-trash', item: FileItem): void;
    (event: 'star', item: FileItem, isStarred: boolean): void;
}>();

const query = ref('');
const mode = ref<FileSearchMode>('named');
const hasSearched = ref(false);

function submitSearch() {
    const trimmedQuery = query.value.trim();

    if (!trimmedQuery || props.isSearching) return;

    hasSearched.value = true;
    emit('search', {
        query: trimmedQuery,
        mode: mode.value,
    });
}

function handleStarToggle(item: FileItem, isStarred: boolean) {
    emit('star', item, isStarred);
}

function clearSearch() {
    query.value = '';
    hasSearched.value = false;
    emit('clear');
}

function changeMode(nextMode: FileSearchMode) {
    if (mode.value === nextMode) return;

    mode.value = nextMode;
    hasSearched.value = false;
    emit('clear');
}

function invalidateResults() {
    if (!hasSearched.value) return;

    hasSearched.value = false;
    emit('clear');
}

function formatFileSize(size?: number | null) {
    if (size == null) return '';

    if (size < 1024) return `${size} B`;
    if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;

    return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}
</script>

<template>
    <section class="search-panel" aria-label="Search files">
        <div class="search-panel-heading">
            <div>
                <p class="search-kicker">FIND SOMETHING</p>
                <h2>Search your files</h2>
            </div>

            <div class="search-mode-selector" role="group" aria-label="Search mode">
                <button class="search-mode-button" :class="{ 'is-active': mode === 'named' }" type="button"
                    :aria-pressed="mode === 'named'" @click="changeMode('named')">
                    Name
                </button>
                <button class="search-mode-button" :class="{ 'is-active': mode === 'smart' }" type="button"
                    :aria-pressed="mode === 'smart'" @click="changeMode('smart')">
                    Smart
                </button>
            </div>
        </div>

        <form class="search-form" @submit.prevent="submitSearch">
            <label class="search-input-wrap" for="file-search-query">
                <span class="search-input-icon" aria-hidden="true">⌕</span>
                <input id="file-search-query" v-model="query" type="search"
                    placeholder="Search by file name or keywords" autocomplete="off" @input="invalidateResults" />
            </label>

            <button class="search-submit" type="submit" :disabled="!query.trim() || isSearching">
                {{ isSearching ? 'Searching…' : 'Search' }}
            </button>
        </form>

        <p class="search-hint">
            {{
                mode === 'named'
                    ? 'Find files by their name.'
                    : 'Find files by describing their contents.'
            }}
        </p>

        <div v-if="hasSearched" class="search-results" aria-live="polite">
            <div class="search-results-heading">
                <div>
                    <h3>{{ mode === 'named' ? 'Name results' : 'Smart results' }}</h3>
                    <p v-if="!isSearching && !error">
                        {{ total }} {{ total === 1 ? 'result' : 'results' }}
                    </p>
                </div>

                <button class="search-clear-button" type="button" @click="clearSearch">
                    Clear
                </button>
            </div>

            <p v-if="isSearching" class="search-state" role="status">
                Searching your files…
            </p>

            <p v-else-if="error" class="search-state search-state-error" role="alert">
                {{ error }}
            </p>

            <p v-else-if="results.length === 0" class="search-state">
                No matching files found. Try another search.
            </p>

            <ul v-else class="search-result-list">
                <li v-for="item in results" :key="item.public_id" class="search-result">
                    <span class="search-result-icon" aria-hidden="true">📄</span>

                    <div class="search-result-info">
                        <span class="search-result-name">{{ item.name }}</span>
                        <span class="search-result-meta">
                            {{ item.extension?.toUpperCase() || item.type }}
                            <template v-if="item.size != null">
                                · {{ formatFileSize(item.size) }}
                            </template>
                        </span>
                    </div>

                    <FileStarButton :item="item" :disabled="props.updatingStarredIds?.includes(item.public_id) ?? false"
                        @toggle="handleStarToggle" />

                    <FileActionsMenu :item="item" @open="emit('open', $event)" @download="emit('download', $event)"
                        @move-to-trash="emit('move-to-trash', $event)" />
                </li>
            </ul>
        </div>
    </section>
</template>

<style src="../../assets/styles/FileSearch.css" scoped></style>