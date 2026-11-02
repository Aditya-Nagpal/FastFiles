<script setup>
import { ref, onMounted } from 'vue';
import { useAuthStore } from './store/auth';
import { useUserStore } from './store/user';
import { useRouter, useRoute } from 'vue-router'
import PUBLIC_PAGES from './router/publicPages.js';

const auth = useAuthStore();
const user = useUserStore();
const router = useRouter();
const route = useRoute();
const isLoading = ref(true);

onMounted(async () => {
  console.log('App mounted');
  await router.isReady();
  const publicPages = PUBLIC_PAGES;
  try {
    await auth.checkAuth();
    console.log('checkAuth success');
    if(!user.user){
      console.log('User not found, fetching user profile...');
      try {
        console.log('Fetching user profile...');
        await user.fetchUserProfile();
        console.log('User profile fetched:', user.user);
      } catch (error) {
        console.error('Error in fetchUserProfile:', error);
      }
    } else {
      console.log('User already exists:', user.user);
    }
    if(publicPages.includes(route.path)){
      router.push('/');
      return;
    }
  } catch (error) {
    console.error('Authentication check failed:', error);
    if(!publicPages.includes(route.path)) {
      router.push('/user/login');
      return;
    }
  } finally {
    isLoading.value = false;
  }
});
</script>

<template>
  <div v-if="isLoading" class="loading-spinner">
    Loading...
  </div>
  <div v-else>
    <router-view />
  </div>
</template>

<style scoped>
.logo {
  height: 6em;
  padding: 1.5em;
  will-change: filter;
  transition: filter 300ms;
}
.logo:hover {
  filter: drop-shadow(0 0 2em #646cffaa);
}
.logo.vue:hover {
  filter: drop-shadow(0 0 2em #42b883aa);
}
.loading-spinner {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 100vh;
  font-size: 2rem;
  font-weight: bold;
}
</style>