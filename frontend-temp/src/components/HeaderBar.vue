<template>
  <nav class="navbar navbar-light bg-white border-bottom px-4 d-flex justify-content-between">
    <h4 class="mt-2 fw-bold"><a href="/" class="mt-0 text-black" style="text-decoration: none;">FastFiles</a></h4>

    <div v-if="isAuthenticated" class="d-flex align-items-center gap-3">
      <div class="dropdown" v-if="displayUploadButton">
        <button
          class="btn btn-primary dropdown-toggle"
          type="button"
          data-bs-toggle="dropdown"
        >
          Upload
        </button>
        <ul class="dropdown-menu">
          <li><a class="dropdown-item mt-0" href="#" @click.prevent="showFileModal = true">Upload File</a></li>
          <li><a class="dropdown-item" href="#" @click.prevent="showFolderModal = true">Upload Folder</a></li>
        </ul>

        <FileModal :show="showFileModal" @close="showFileModal = false" />
        <FolderModal :show="showFolderModal" @close="showFolderModal = false" />

      </div>

      <div class="dropdown">
        <img
          :src="displayPictureUrl"
          class="rounded-circle"
          style="width: 40px; height: 40px; cursor: pointer;"
          data-bs-toggle="dropdown"
        />
        <ul class="dropdown-menu dropdown-menu-end">
          <li><a class="dropdown-item mt-0" href="/user/profile">View Profile</a></li>
          <li><a class="dropdown-item" href="#" @click="handleLogout">Logout</a></li>
        </ul>
      </div>
    </div>

    <div v-else>
      <button class="btn btn-outline-primary me-2" @click="router.push('/user/login')">
        Login
      </button>
      <button class="btn btn-primary" @click="router.push('/user/signup')">
        Sign Up
      </button>
    </div>
  </nav>
</template>

<script setup>
import { ref, computed } from 'vue';
import { useRoute, useRouter } from 'vue-router'
import FolderModal from './FolderModal.vue';
import FileModal from './FileModal.vue';
import defaultImage from '../images/person.png';
import { useAuthStore } from '../store/auth.js';
import { useUserStore } from '../store/user.js';

const auth = useAuthStore();
const userStore = useUserStore();
const router = useRouter();
const route = useRoute();

const showFileModal = ref(false);
const showFolderModal = ref(false);

const user = computed(() => userStore.user);
const isAuthenticated = computed(() => auth.isAuthenticated);
const displayPictureUrl = computed(() => {
  return user?.value?.display_picture || defaultImage;
});
const displayUploadButton = computed(() => {
  return isAuthenticated.value && route.path === '/';
});

const handleLogout = async () => {
  try {
    await auth.logout();
    router.push('/user/login');
  } catch (error) {
    console.error('logout failed: ', error);
    return;
  }
};

</script>

<style scoped>
.navbar {
  position: sticky;
  top: 0;
  z-index: 1000;
}
</style>