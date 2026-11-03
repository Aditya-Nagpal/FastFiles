<script setup lang="ts">
import { onMounted, ref } from 'vue';

type Theme = 'light' | 'dark';

const theme = ref<Theme>('light');

function applyTheme(value: Theme) {
  theme.value = value;
  document.documentElement.dataset.theme = value;
  localStorage.setItem('fastfiles-theme', value);
}

function toggleTheme() {
  applyTheme(theme.value === 'light' ? 'dark' : 'light');
}

onMounted(() => {
  const savedTheme = localStorage.getItem('fastfiles-theme');
  applyTheme(savedTheme === 'dark' ? 'dark' : 'light');
});
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <a class="brand" href="/" aria-label="FastFiles home">
        <span class="brand-mark">F</span>
        <span>FastFiles</span>
      </a>

      <div class="sidebar-section">
        <p class="section-label">Workspace</p>

        <nav class="navigation" aria-label="Main navigation">
          <div class="nav-item is-active" aria-current="page">
            <span class="nav-icon">▤</span>
            <span>Files</span>
          </div>
          <div class="nav-item">
            <span class="nav-icon">♧</span>
            <span>Shared</span>
          </div>
          <div class="nav-item">
            <span class="nav-icon">◷</span>
            <span>Recent</span>
          </div>
          <div class="nav-item">
            <span class="nav-icon">☆</span>
            <span>Starred</span>
          </div>
          <div class="nav-item">
            <span class="nav-icon">▱</span>
            <span>Trash</span>
          </div>
        </nav>
      </div>

      <div class="storage-card">
        <div class="storage-icon">◇</div>
        <p class="storage-title">Storage</p>
        <p class="storage-copy">Usage details will appear when we connect your file service.</p>
      </div>

      <div class="sidebar-bottom">
        <span class="avatar">U</span>
        <div>
          <p class="account-title">Your account</p>
          <p class="account-subtitle">Profile settings</p>
        </div>
      </div>
    </aside>

    <main class="main-area">
      <header class="topbar">
        <div class="breadcrumb">
          <span>Workspace</span>
          <span class="breadcrumb-divider">/</span>
          <strong>Files</strong>
        </div>

        <button
          class="theme-button"
          type="button"
          :aria-label="theme === 'light' ? 'Switch to dark mode' : 'Switch to light mode'"
          @click="toggleTheme"
        >
          <span aria-hidden="true">{{ theme === 'light' ? '☾' : '☀' }}</span>
          <span>{{ theme === 'light' ? 'Dark mode' : 'Light mode' }}</span>
        </button>
      </header>

      <section class="content">
        <div class="page-heading">
          <div>
            <p class="eyebrow">YOUR WORKSPACE</p>
            <h1>Your files</h1>
            <p class="page-description">
              A calmer place to keep everything organized.
            </p>
          </div>

          <div class="heading-actions">
            <button class="button button-secondary" type="button" disabled>
              <span aria-hidden="true">＋</span>
              New folder
            </button>
            <button class="button button-primary" type="button" disabled>
              <span aria-hidden="true">↑</span>
              Upload
            </button>
          </div>
        </div>

        <section class="workspace-card" aria-labelledby="empty-heading">
          <div class="empty-icon" aria-hidden="true">▤</div>
          <p class="eyebrow">READY WHEN YOU ARE</p>
          <h2 id="empty-heading">Your workspace is taking shape</h2>
          <p>
            Your folders and files will appear here after we connect FastFiles
            to your API gateway.
          </p>
        </section>

        <footer class="page-footer">
          <span>FastFiles</span>
          <span>Secure file storage, made simple.</span>
        </footer>
      </section>
    </main>
  </div>
</template>

<style scoped>
.app-shell {
  display: flex;
  min-height: 100vh;
  background: var(--color-bg);
}

.sidebar {
  display: flex;
  flex: 0 0 248px;
  flex-direction: column;
  min-height: 100vh;
  padding: 28px 18px 20px;
  background: var(--color-surface);
  border-right: 1px solid var(--color-border);
}

.brand {
  display: flex;
  align-items: center;
  gap: 11px;
  margin: 0 0 48px 5px;
  color: var(--color-text);
  font-size: 19px;
  font-weight: 750;
  letter-spacing: -0.5px;
  text-decoration: none;
}

.brand-mark {
  display: grid;
  width: 34px;
  height: 34px;
  place-items: center;
  border-radius: 11px;
  background: var(--color-primary);
  color: var(--color-on-primary);
  font-size: 18px;
  font-weight: 800;
}

.sidebar-section {
  flex: 1;
}

.section-label,
.eyebrow {
  margin: 0;
  color: var(--color-text-subtle);
  font-size: 10px;
  font-weight: 750;
  letter-spacing: 1.2px;
}

.section-label {
  padding: 0 12px;
  margin-bottom: 12px;
}

.navigation {
  display: grid;
  gap: 5px;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 13px;
  min-height: 43px;
  padding: 0 12px;
  border-radius: 10px;
  color: var(--color-text-muted);
  font-size: 14px;
  font-weight: 550;
}

.nav-item.is-active {
  background: var(--color-primary-soft);
  color: var(--color-primary);
  font-weight: 700;
}

.nav-icon {
  display: inline-grid;
  width: 20px;
  place-items: center;
  font-size: 18px;
}

.storage-card {
  padding: 16px;
  margin-top: 30px;
  border: 1px solid var(--color-border);
  border-radius: 14px;
  background: var(--color-surface-soft);
}

.storage-icon {
  color: var(--color-primary);
  font-size: 22px;
}

.storage-title,
.account-title {
  margin: 8px 0 4px;
  color: var(--color-text);
  font-size: 13px;
  font-weight: 700;
}

.storage-copy,
.account-subtitle {
  margin: 0;
  color: var(--color-text-muted);
  font-size: 12px;
  line-height: 1.5;
}

.sidebar-bottom {
  display: flex;
  align-items: center;
  gap: 11px;
  padding: 18px 5px 0;
  margin-top: 18px;
  border-top: 1px solid var(--color-border);
}

.avatar {
  display: grid;
  width: 36px;
  height: 36px;
  place-items: center;
  border-radius: 50%;
  background: var(--color-primary-soft);
  color: var(--color-primary);
  font-size: 13px;
  font-weight: 750;
}

.account-title {
  margin: 0 0 3px;
}

.main-area {
  flex: 1;
  min-width: 0;
}

.topbar {
  display: flex;
  min-height: 76px;
  align-items: center;
  justify-content: space-between;
  padding: 0 42px;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-surface);
}

.breadcrumb {
  display: flex;
  align-items: center;
  gap: 11px;
  color: var(--color-text-muted);
  font-size: 13px;
}

.breadcrumb strong {
  color: var(--color-text);
  font-weight: 650;
}

.breadcrumb-divider {
  color: var(--color-text-subtle);
}

.theme-button,
.button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 9px;
  min-height: 40px;
  padding: 0 14px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
  background: var(--color-surface);
  color: var(--color-text);
  font-size: 13px;
  font-weight: 650;
}

.theme-button {
  cursor: pointer;
}

.theme-button:hover {
  background: var(--color-surface-hover);
}

.content {
  width: min(1120px, 100%);
  padding: 46px 42px 28px;
  margin: 0 auto;
}

.page-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  margin-bottom: 30px;
}

.page-heading h1 {
  margin: 9px 0 7px;
  color: var(--color-text);
  font-size: clamp(28px, 4vw, 36px);
  letter-spacing: -1.2px;
}

.page-description {
  margin: 0;
  color: var(--color-text-muted);
  font-size: 14px;
}

.heading-actions {
  display: flex;
  gap: 10px;
}

.button {
  min-height: 43px;
}

.button-primary {
  border-color: var(--color-primary);
  background: var(--color-primary);
  color: var(--color-on-primary);
}

.button-secondary {
  background: var(--color-surface);
}

.button:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.workspace-card {
  display: flex;
  min-height: 370px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 32px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-card);
  background: var(--color-surface);
  box-shadow: var(--shadow-card);
  text-align: center;
}

.empty-icon {
  display: grid;
  width: 58px;
  height: 58px;
  place-items: center;
  margin-bottom: 20px;
  border-radius: 18px;
  background: var(--color-primary-soft);
  color: var(--color-primary);
  font-size: 27px;
}

.workspace-card h2 {
  margin: 12px 0 8px;
  color: var(--color-text);
  font-size: 20px;
  letter-spacing: -0.3px;
}

.workspace-card > p:last-child {
  max-width: 390px;
  margin: 0;
  color: var(--color-text-muted);
  font-size: 14px;
  line-height: 1.65;
}

.page-footer {
  display: flex;
  justify-content: space-between;
  padding: 22px 2px 0;
  color: var(--color-text-subtle);
  font-size: 12px;
}

@media (max-width: 760px) {
  .sidebar {
    flex-basis: 76px;
    align-items: center;
    padding: 20px 10px;
  }

  .brand {
    margin: 0 0 38px;
  }

  .brand > span:last-child,
  .section-label,
  .nav-item > span:last-child,
  .storage-card,
  .sidebar-bottom > div {
    display: none;
  }

  .navigation {
    gap: 8px;
  }

  .nav-item {
    width: 48px;
    justify-content: center;
    padding: 0;
  }

  .nav-icon {
    font-size: 20px;
  }

  .sidebar-bottom {
    padding: 18px 0 0;
  }

  .topbar {
    min-height: 64px;
    padding: 0 20px;
  }

  .content {
    padding: 32px 20px 24px;
  }

  .page-heading {
    align-items: flex-start;
    flex-direction: column;
  }
}

@media (max-width: 480px) {
  .theme-button {
    width: 40px;
    padding: 0;
    font-size: 0;
  }

  .page-heading {
    gap: 18px;
  }

  .heading-actions {
    width: 100%;
  }

  .heading-actions .button {
    flex: 1;
  }

  .workspace-card {
    min-height: 310px;
    padding: 24px;
  }

  .page-footer {
    flex-direction: column;
    gap: 6px;
  }
}
</style>