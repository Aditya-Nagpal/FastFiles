<template>
  <HeaderBar />
  <div class="signup-page d-flex flex-column align-items-center py-5">
    <h2 class="text-center mb-5 fw-semibold">Create Your FastFiles Account</h2>
    <SignUpForm />    
  </div>
</template>

<script setup>
import HeaderBar from '../components/HeaderBar.vue'; 
import SignUpForm from '../components/SignUpForm.vue';

</script>

<style scoped>
.signup-page {
  background-color: #fcfcfd; 
  min-height: 100vh;
  padding-bottom: 5rem !important; 
}
</style>
