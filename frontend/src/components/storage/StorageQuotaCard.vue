<script setup lang="ts">
import { computed, onMounted } from 'vue';
import { useStorageQuotaStore } from '../../stores/storageQuota';

const quotaStore = useStorageQuotaStore();
const quota = computed(() => quotaStore.quota);

const segmentDefinitions = [
    { key: 'activeBytes', label: 'Active', colorClass: 'active' },
    { key: 'trashBytes', label: 'Trash', colorClass: 'trash' },
    { key: 'reservedBytes', label: 'Reserved', colorClass: 'reserved' },
] as const;

const occupiedBytes = computed(() => {
    if (!quota.value) return 0;

    return (
        quota.value.activeBytes +
        quota.value.trashBytes +
        quota.value.reservedBytes
    );
});

const occupiedPercentage = computed(() => {
    if (!quota.value || quota.value.quotaBytes <= 0) return 0;

    return Math.min(
        100,
        (occupiedBytes.value / quota.value.quotaBytes) * 100,
    );
});

const percentageLabel = computed(() => {
    const value = occupiedPercentage.value;

    if (value < 1) return value.toFixed(2);
    if (value < 10) return value.toFixed(1);
    return Math.round(value).toString();
});

const segments = computed(() => {
    if (!quota.value || quota.value.quotaBytes <= 0) return [];

    let remainingPercentage = 100;

    return segmentDefinitions.map((definition) => {
        const bytes = quota.value![definition.key];
        const percentage = Math.min(
            remainingPercentage,
            (bytes / quota.value!.quotaBytes) * 100,
        );

        remainingPercentage -= percentage;

        return {
            ...definition,
            bytes,
            percentage,
        };
    });
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
            <div class="storage-progress" role="img" :aria-label="`${percentageLabel}% of storage allocated`">
                <span v-for="segment in segments" :key="segment.key" class="storage-progress-segment"
                    :class="`storage-progress-segment--${segment.colorClass}`"
                    :style="{ width: `${segment.percentage}%` }"
                    :title="`${segment.label}: ${formatBytes(segment.bytes)}`" />
            </div>

            <div class="storage-usage-labels">
                <span>{{ percentageLabel }}% allocated</span>
                <span>{{ formatBytes(quota.quotaBytes) }}</span>
            </div>

            <ul class="storage-breakdown" aria-label="Storage breakdown">
                <li v-for="segment in segments" :key="segment.key">
                    <span class="storage-breakdown-label">
                        <span class="storage-breakdown-dot" :class="`storage-breakdown-dot--${segment.colorClass}`"
                            aria-hidden="true" />
                        {{ segment.label }}
                    </span>
                    <span>{{ formatBytes(segment.bytes) }}</span>
                </li>
            </ul>

            <p class="storage-available">
                {{ formatBytes(quota.availableBytes) }} available
            </p>
        </div>
    </section>
</template>

<style src="../../assets/styles/StorageQuotaCard.css" scoped></style>