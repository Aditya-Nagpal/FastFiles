<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useAuthStore } from '../../stores/auth';
import { useUserStore } from '../../stores/user';
import StorageQuotaCard from '../storage/StorageQuotaCard.vue';

const authStore = useAuthStore();
const userStore = useUserStore();
const router = useRouter();
const isSigningOut = ref(false);

const profile = computed(() => userStore.user);
const profileInitial = computed(
  () => profile.value?.name?.trim().charAt(0).toUpperCase() || 'U',
);

onMounted(async () => {
  if (userStore.user) return;

  try {
    await userStore.fetchUserProfile();
  } catch (error) {
    console.error('Could not load the sidebar profile:', error);
  }
});

async function signOut() {
  if (isSigningOut.value) return;

  isSigningOut.value = true;

  try {
    await authStore.logout();
  } catch (error) {
    console.error('Sign-out request failed:', error);
  } finally {
    userStore.clearProfile();
    await router.replace({ name: 'login' });
    isSigningOut.value = false;
  }
}
</script>

<template>
  <aside class="sidebar">
    <RouterLink class="brand" to="/" aria-label="FastFiles home">
      <span class="brand-mark">F</span>
      <span>FastFiles</span>
    </RouterLink>

    <div class="sidebar-section">
      <p class="section-label">Workspace</p>

      <nav class="navigation" aria-label="Main navigation">
        <RouterLink class="nav-item" to="/" exact-active-class="is-active">
          <span class="nav-icon">▤</span>
          <span>Files</span>
        </RouterLink>
        <div class="nav-item">
          <span class="nav-icon">♧</span>
          <span>Shared</span>
        </div>
        <div class="nav-item">
          <span class="nav-icon">◷</span>
          <span>Recent</span>
        </div>
        <RouterLink class="nav-item" to="/starred" exact-active-class="is-active">
          <span class="nav-icon">☆</span>
          <span>Starred</span>
        </RouterLink>
        <RouterLink class="nav-item" to="/trash" exact-active-class="is-active">
          <span class="nav-icon">▱</span>
          <span>Trash</span>
        </RouterLink>
      </nav>
    </div>

    <StorageQuotaCard />

    <div class="sidebar-account">
      <RouterLink class="profile-link" :to="{ name: 'profile' }" aria-label="Open profile settings"
        title="Profile settings">
        <img v-if="profile?.display_picture" class="avatar avatar-image" :src="profile.display_picture"
          alt="Profile picture" />
        <span v-else class="avatar">{{ profileInitial }}</span>

        <span class="profile-link-copy">
          <span class="account-title">{{ profile?.name || 'Your account' }}</span>
          <span class="account-subtitle">Profile settings</span>
        </span>
      </RouterLink>

      <button class="sidebar-signout" type="button" :disabled="isSigningOut" title="Sign out" @click="signOut">
        <span class="signout-icon" aria-hidden="true">⇥</span>
        <span class="signout-label">
          {{ isSigningOut ? 'Signing out…' : 'Sign out' }}
        </span>
      </button>
    </div>
  </aside>
</template>

<style src="../../assets/styles/AppSidebar.css" scoped></style>