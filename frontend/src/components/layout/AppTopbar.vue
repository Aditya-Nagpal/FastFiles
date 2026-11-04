<script setup lang="ts">
type Theme = 'light' | 'dark';

const props = defineProps<{
  theme: Theme;
}>();

const emit = defineEmits<{
  (event: 'toggle-theme'): void;
}>();
</script>

<template>
  <header class="topbar">
    <div class="breadcrumb">
      <span>Workspace</span>
      <span class="breadcrumb-divider">/</span>
      <strong>Files</strong>
    </div>

    <button
      class="theme-button"
      type="button"
      :aria-label="props.theme === 'light' ? 'Switch to dark mode' : 'Switch to light mode'"
      @click="emit('toggle-theme')"
    >
      <span aria-hidden="true">{{ props.theme === 'light' ? '☾' : '☀' }}</span>
      <span>{{ props.theme === 'light' ? 'Dark mode' : 'Light mode' }}</span>
    </button>
  </header>
</template>

<style src="../../assets/styles/AppTopbar.css" scoped></style>