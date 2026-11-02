import { createApp } from 'vue';
import './assets/style.css';
import router from './router';
import App from './App.vue';
import { createPinia } from 'pinia';
import Vue3Toastity from 'vue3-toastify';
import 'vue3-toastify/dist/index.css';
import 'bootstrap/dist/css/bootstrap.min.css'
import 'bootstrap/dist/js/bootstrap.bundle.min.js'
import 'bootstrap-icons/font/bootstrap-icons.css'

const pinia = createPinia();

createApp(App).use(pinia).use(router).use(Vue3Toastity, { autoClose: 3000 }).mount('#app')