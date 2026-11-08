<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import type { FileItem } from '../stores/files';
import { useFilesStore } from '../stores/files';
import { useStarredStore } from '../stores/starred';
import { useStorageQuotaStore } from '../stores/storageQuota';
import AppSidebar from '../components/layout/AppSidebar.vue';
import AppTopbar from '../components/layout/AppTopbar.vue';
import AppFooter from '../components/layout/AppFooter.vue';
import FileList from '../components/files/FileList.vue';
import ConfirmDeleteModal from '../components/files/ConfirmDeleteModal.vue';
import FileInfoModal from '../components/files/FileInfoModal.vue';

type Theme = 'light' | 'dark';

const router = useRouter();
const files = useFilesStore();
const starred = useStarredStore();
const storageQuota = useStorageQuotaStore();

const theme = ref<Theme>('light');
const downloadingFileId = ref<string | null>(null);
const itemToMoveToTrash = ref<FileItem | null>(null);
const itemForInfo = ref<FileItem | null>(null);
const isMovingToTrash = ref(false);
const moveToTrashError = ref('');
const actionError = ref('');

function applyTheme(value: Theme) {
    theme.value = value;
    document.documentElement.dataset.theme = value;
    localStorage.setItem('fastfiles-theme', value);
}

function toggleTheme() {
    applyTheme(theme.value === 'light' ? 'dark' : 'light');
}

async function loadStarred() {
    actionError.value = '';

    try {
        await starred.fetchStarred();
    } catch {
        // The store exposes the load error to the page.
    }
}

async function toggleStar(item: FileItem, isStarred: boolean) {
    actionError.value = '';

    try {
        await starred.setStarred(item, isStarred);
    } catch (error) {
        console.error('Could not update starred item:', error);
        actionError.value = 'Could not update this starred item. Please try again.';
    }
}

async function openFolder(item: FileItem) {
    if (item.type !== 'FOLDER') return;

    actionError.value = '';
    files.parentId = null;
    files.currentKey = '';
    files.keyStack = [];

    try {
        await files.enterFolder(item);
        await router.push('/');
    } catch {
        actionError.value = 'Could not open this folder. Please try again.';
    }
}

async function downloadFile(item: FileItem) {
    downloadingFileId.value = item.public_id;
    actionError.value = '';

    try {
        await files.downloadFile(item.public_id);
    } catch (error) {
        console.error('Download failed:', error);
        actionError.value = 'Could not download this item. Please try again.';
    } finally {
        downloadingFileId.value = null;
    }
}

function requestMoveToTrash(item: FileItem) {
    itemToMoveToTrash.value = item;
    moveToTrashError.value = '';
}

function closeMoveToTrashModal() {
    if (isMovingToTrash.value) return;

    itemToMoveToTrash.value = null;
    moveToTrashError.value = '';
}

async function confirmMoveToTrash() {
    if (!itemToMoveToTrash.value) return;

    isMovingToTrash.value = true;
    moveToTrashError.value = '';

    try {
        const publicId = itemToMoveToTrash.value.public_id;
        await files.moveToTrash(publicId);

        const wasStarred = starred.items.some(
            (item) => item.public_id === publicId,
        );

        starred.items = starred.items.filter(
            (item) => item.public_id !== publicId,
        );

        if (wasStarred) {
            starred.total = Math.max(0, starred.total - 1);
        }

        itemToMoveToTrash.value = null;

        void storageQuota.fetchQuota().catch((error) => {
            console.error('Could not refresh storage quota:', error);
        });
    } catch (error) {
        console.error('Move to trash failed:', error);
        moveToTrashError.value = 'Could not move this item to Trash. Please try again.';
    } finally {
        isMovingToTrash.value = false;
    }
}

function showInfo(item: FileItem) {
    itemForInfo.value = item;
}

onMounted(() => {
    const savedTheme = localStorage.getItem('fastfiles-theme');
    applyTheme(savedTheme === 'dark' ? 'dark' : 'light');
    void loadStarred();
});
</script>

<template>
    <div class="app-shell">
        <AppSidebar />

        <main class="main-area">
            <AppTopbar :theme="theme" @toggle-theme="toggleTheme" />

            <section class="content">
                <div class="page-heading">
                    <div>
                        <p class="eyebrow">YOUR WORKSPACE</p>
                        <h1>Starred</h1>
                        <p class="page-description">
                            Files and folders you’ve saved for quick access.
                        </p>
                    </div>
                </div>

                <p v-if="actionError" class="upload-error" role="alert">
                    {{ actionError }}
                </p>

                <section class="workspace-card" aria-label="Starred files and folders">
                    <FileList :items="starred.items" :total="starred.total" :is-loading="starred.isLoading"
                        :error="starred.errorMessage" current-key="Starred" :can-go-back="false"
                        :downloading-file-id="downloadingFileId" :updating-starred-ids="starred.updatingPublicIds"
                        empty-title="Nothing starred yet" empty-description="Star a file or folder to find it here."
                        @retry="loadStarred" @open="openFolder" @download="downloadFile"
                        @move-to-trash="requestMoveToTrash" @info="showInfo" @star="toggleStar" />

                    <ConfirmDeleteModal :open="itemToMoveToTrash !== null" :item="itemToMoveToTrash"
                        :is-deleting="isMovingToTrash" :error="moveToTrashError" @close="closeMoveToTrashModal"
                        @confirm="confirmMoveToTrash" />

                    <FileInfoModal :open="itemForInfo !== null" :item="itemForInfo" @close="itemForInfo = null" />
                </section>

                <AppFooter />
            </section>
        </main>
    </div>
</template>

<style src="../assets/styles/HomeView.css" scoped></style>