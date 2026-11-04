<script setup lang="ts">
import { computed, onMounted } from 'vue';
import { useStorageQuotaStore } from '../../stores/storageQuota';

const quotaStore = useStorageQuotaStore();
const quota = computed(() => quotaStore.quota);

const progressPercentage = computed(() => {
    const value = quota.value?.usagePercentage ?? 0;
    return Math.min(100, Math.max(0, value));
});

const percentageLabel = computed(() => {
    const value = progressPercentage.value;
    if (value < 1) return value.toFixed(2);
    if (value < 10) return value.toFixed(1);
    return Math.round(value).toString();
});

function formatBytes(bytes: number) {
    if (bytes < 1024) return `${bytes} B`;

    const units = ['KB', 'MB', 'GB', 'TB'];
    let value = bytes / 1024;
    let unitIndex = 0;

    while (value >= 1024 && unitIndex < units.length - 1) {
        value /= 1024;
        unitIndex += 1;
    }

    return `${value.toFixed(1)} ${units[unitIndex]}`;
}

async function loadQuota() {
    try {
        await quotaStore.fetchQuota();
    } catch {
        // The card displays its own error and retry action.
    }
}

onMounted(() => {
    void loadQuota();
});
</script>

<template>
    <section class="storage-card" aria-label="Storage usage">
        <div class="storage-heading">
            <span class="storage-icon" aria-hidden="true">◇</span>
            <div>
                <p class="storage-title">Storage</p>
                <p class="storage-subtitle">Your usage</p>
            </div>
        </div>

        <p v-if="quotaStore.isLoading && !quota" class="storage-state" role="status">
            Loading usage…
        </p>

        <div v-else-if="quotaStore.errorMessage && !quota" class="storage-state" role="alert">
            <p>{{ quotaStore.errorMessage }}</p>
            <button class="storage-retry" type="button" @click="loadQuota">
                Try again
            </button>
        </div>

        <div v-else-if="quota" class="storage-usage">
            <div class="storage-progress" role="progressbar" aria-label="Storage used"
                :aria-valuenow="progressPercentage" aria-valuemin="0" aria-valuemax="100">
                <span class="storage-progress-fill" :style="{ width: `${progressPercentage}%` }" />
            </div>

            <div class="storage-usage-labels">
                <span>{{ percentageLabel }}% used</span>
                <span>{{ formatBytes(quota.quotaBytes) }}</span>
            </div>

            <p class="storage-copy">
                {{ formatBytes(quota.usedBytes) }} used
            </p>
            <p class="storage-available">
                {{ formatBytes(quota.availableBytes) }} available
            </p>
        </div>
    </section>
</template>

<style src="../../assets/styles/StorageQuotaCard.css" scoped></style>