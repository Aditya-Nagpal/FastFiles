<template>
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
      <p class="storage-copy">
        Usage details will appear when we connect your file service.
      </p>
    </div>

    <div class="sidebar-bottom">
      <span class="avatar">U</span>
      <div>
        <p class="account-title">Your account</p>
        <p class="account-subtitle">Profile settings</p>
      </div>
    </div>
  </aside>
</template>

<style src="../../assets/styles/AppSidebar.css" scoped></style>