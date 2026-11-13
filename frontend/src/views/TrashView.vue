<script setup lang="ts">
import { onMounted, ref } from 'vue';
import type { FileItem } from '../stores/files';
import { useTrashStore } from '../stores/trash';
import { useStarredStore } from '../stores/starred';
import { useStorageQuotaStore } from '../stores/storageQuota';
import AppSidebar from '../components/layout/AppSidebar.vue';
import AppTopbar from '../components/layout/AppTopbar.vue';
import AppFooter from '../components/layout/AppFooter.vue';
import FileList from '../components/files/FileList.vue';
import FileInfoModal from '../components/files/FileInfoModal.vue';
import ConfirmPermanentDeleteModal from '../components/files/ConfirmPermanentDeleteModal.vue';

type Theme = 'light' | 'dark';

const trash = useTrashStore();
const starred = useStarredStore();
const storageQuota = useStorageQuotaStore();

const theme = ref<Theme>('light');
const restoringPublicIds = ref<string[]>([]);
const permanentlyDeletingPublicIds = ref<string[]>([]);
const actionError = ref('');
const itemForInfo = ref<FileItem | null>(null);
const itemToPermanentlyDelete = ref<FileItem | null>(null);
const permanentDeleteError = ref('');

const isPermanentlyDeleting = ref(false);

function applyTheme(value: Theme) {
    theme.value = value;
    document.documentElement.dataset.theme = value;
    localStorage.setItem('fastfiles-theme', value);
}

function toggleTheme() {
    applyTheme(theme.value === 'light' ? 'dark' : 'light');
}

async function loadTrash() {
    actionError.value = '';

    try {
        await trash.fetchTrash();
    } catch {
        // The store exposes the list error to FileList.
    }
}

async function openFolder(folder: FileItem) {
    actionError.value = '';

    try {
        await trash.enterFolder(folder);
    } catch {
        actionError.value = 'Could not open this folder in Trash. Please try again.';
    }
}

async function goBack() {
    actionError.value = '';

    try {
        await trash.goBack();
    } catch {
        actionError.value = 'Could not load this Trash folder. Please try again.';
    }
}

async function restoreItem(item: FileItem) {
    if (restoringPublicIds.value.includes(item.public_id)) return;

    restoringPublicIds.value.push(item.public_id);
    actionError.value = '';

    try {
        await trash.restoreItem(item.public_id);

        if (item.is_starred) {
            void starred.fetchStarred().catch((error) => {
                console.error('Could not refresh starred items:', error);
            });
        }
    } catch (error) {
        console.error('Could not restore item:', error);
        actionError.value = 'Could not restore this item. Please try again.';
    } finally {
        restoringPublicIds.value = restoringPublicIds.value.filter(
            (id) => id !== item.public_id,
        );
    }
}

function requestPermanentDelete(item: FileItem) {
    itemToPermanentlyDelete.value = item;
    permanentDeleteError.value = '';
}

function closePermanentDeleteModal() {
    if (isPermanentlyDeleting.value) return;

    itemToPermanentlyDelete.value = null;
    permanentDeleteError.value = '';
}

async function confirmPermanentDelete() {
    const item = itemToPermanentlyDelete.value;
    if (!item || isPermanentlyDeleting.value) return;

    const publicId = item.public_id;
    isPermanentlyDeleting.value = true;
    permanentlyDeletingPublicIds.value.push(publicId);
    permanentDeleteError.value = '';

    try {
        await trash.permanentlyDeleteItem(publicId);

        if (item.is_starred) {
            starred.items = starred.items.filter(
                (starredItem) => starredItem.public_id !== publicId,
            );
        }

        itemToPermanentlyDelete.value = null;

        void storageQuota.fetchQuota().catch((error) => {
            console.error('Could not refresh storage quota:', error);
        });
    } catch (error) {
        console.error('Could not permanently delete item:', error);
        permanentDeleteError.value =
            'Could not permanently delete this item. Please try again.';
    } finally {
        isPermanentlyDeleting.value = false;
        permanentlyDeletingPublicIds.value =
            permanentlyDeletingPublicIds.value.filter((id) => id !== publicId);
    }
}

function showInfo(item: FileItem) {
    itemForInfo.value = item;
}

onMounted(() => {
    const savedTheme = localStorage.getItem('fastfiles-theme');
    applyTheme(savedTheme === 'dark' ? 'dark' : 'light');
    void trash.openRoot();
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
                        <h1>Trash</h1>
                        <p class="page-description">
                            Browse trashed files and folders, and restore items you need.
                        </p>
                    </div>
                </div>

                <p v-if="actionError" class="upload-error" role="alert">
                    {{ actionError }}
                </p>

                <section class="workspace-card" aria-label="Trash">
                    <FileList :items="trash.items" :total="trash.total" :is-loading="trash.isLoading"
                        :error="trash.errorMessage" :current-key="trash.currentKey || 'Trash'"
                        :can-go-back="trash.keyStack.length > 0" :downloading-file-id="null"
                        :restoring-public-ids="restoringPublicIds"
                        :permanently-deleting-public-ids="permanentlyDeletingPublicIds" variant="trash"
                        empty-title="Trash is empty" empty-description="Items you move to Trash will appear here."
                        @back="goBack" @retry="loadTrash" @open="openFolder" @restore="restoreItem"
                        @permanent-delete="requestPermanentDelete" @info="showInfo" />

                    <FileInfoModal :open="itemForInfo !== null" :item="itemForInfo" @close="itemForInfo = null" />

                    <ConfirmPermanentDeleteModal :open="itemToPermanentlyDelete !== null"
                        :item="itemToPermanentlyDelete" :is-deleting="isPermanentlyDeleting"
                        :error="permanentDeleteError" @close="closePermanentDeleteModal"
                        @confirm="confirmPermanentDelete" />
                </section>

                <AppFooter />
            </section>
        </main>
    </div>
</template>

<style src="../assets/styles/HomeView.css" scoped></style>