import axios from 'axios';
import { useAuthStore } from '../store/auth';

const BASE_URL = import.meta.env.VITE_API_BASE_URL;
const REFRESH_API = BASE_URL+'/auth/refresh';

// Flag to avoid multiple refresh attempts
let isRefreshing = false;
let failedQueue = [];

const processQueue = (error, token = null) => {
    failedQueue.forEach(prom => {
        if (error) {
            prom.reject(error);
        } else {
            prom.resolve(token);
        }
    });
    failedQueue = [];
};

const API = axios.create({
    baseURL: BASE_URL,
    withCredentials: true
});

// Intercept 401 responses (unauthorized) and refresh token
API.interceptors.response.use(
    response => response,
    async (error) => {
        const originalRequest = error.config;

        // Check for 401
        if(error.response?.status === 401 && !originalRequest._retry){
            originalRequest._retry = true;
            const authStore = useAuthStore();
            if(isRefreshing) {
                return new Promise((resolve, reject) => {
                    failedQueue.push({resolve, reject});
                })
                    .then((token) => {
                        originalRequest.headers.Authorization = 'Bearer '+token;
                        return API(originalRequest);
                    })
                    .catch((err) => Promise.reject(err));
            }

            isRefreshing = true;

            try {
                // const newToken = await getNewToken(); // Calls /auth/refresh
                const res = await axios.get(REFRESH_API, {
                    withCredentials: true // sends HttpOnly cookie
                });
                console.log('Token refreshed:', res.data);
                const newToken = res.data.accessToken;
                authStore.setAccessToken(newToken);

                API.defaults.headers.common.Authorization = 'Bearer '+newToken;
                originalRequest.headers.Authorization = 'Bearer '+newToken;

                processQueue(null, newToken);
                return API(originalRequest);
            } catch (error) {
                processQueue(error, null);
                await authStore.logout();
                return Promise.reject(error);
            } finally {
                isRefreshing = false;
            }
        }
        
        return Promise.reject(error);
    }
);

// Request interceptor - attact access token
API.interceptors.request.use((config) => {
    const authStore = useAuthStore();
    if (authStore.accessToken) {
        config.headers.Authorization = `Bearer ${authStore.accessToken}`;
    }
    return config;
});

export default API;