import axios, {
    type AxiosError,
    type InternalAxiosRequestConfig,
} from 'axios';
import { appConfig } from '../config/env';
import { apiRoutes } from './routes';
import { pinia } from '../stores/pinia';
import { useAuthStore } from '../stores/auth';

type RetryConfig = InternalAxiosRequestConfig & {
    _retry?: boolean;
};

interface RefreshResponse {
    accessToken?: string;
}

export const apiClient = axios.create({
    baseURL: appConfig.apiGatewayUrl,
    withCredentials: true,
    headers: {
        Accept: 'application/json',
    },
});

// This separate client prevents refresh requests from re-entering
// the apiClient's 401 interceptor.
const refreshClient = axios.create({
    baseURL: appConfig.apiGatewayUrl,
    withCredentials: true,
});

let refreshPromise: Promise<string> | null = null;

function shouldSkipRefresh(url?: string): boolean {
    if (!url) return false;

    if (url.includes(`${apiRoutes.auth}/forgot-password/`)) {
        return true;
    }

    return [
        `${apiRoutes.auth}/login`,
        `${apiRoutes.auth}/register`,
        `${apiRoutes.auth}/refresh`,
    ].some((path) => url.endsWith(path));
}

function getFreshAccessToken(): Promise<string> {
    if (!refreshPromise) {
        refreshPromise = refreshClient
            .get<RefreshResponse>(`${apiRoutes.auth}/refresh`)
            .then(({ data }) => {
                if (!data.accessToken) {
                    throw new Error('The refresh response did not include an access token.');
                }

                return data.accessToken;
            })
            .finally(() => {
                refreshPromise = null;
            });
    }

    return refreshPromise;
}

apiClient.interceptors.request.use((config) => {
    const auth = useAuthStore(pinia);

    if (auth.accessToken) {
        config.headers.set('Authorization', `Bearer ${auth.accessToken}`);
    }

    return config;
});

apiClient.interceptors.response.use(
    (response) => response,
    async (error: AxiosError) => {
        const originalRequest = error.config as RetryConfig | undefined;

        if (
            error.response?.status !== 401 ||
            !originalRequest ||
            originalRequest._retry ||
            shouldSkipRefresh(originalRequest.url)
        ) {
            return Promise.reject(error);
        }

        originalRequest._retry = true;

        try {
            const accessToken = await getFreshAccessToken();
            const auth = useAuthStore(pinia);

            auth.setAccessToken(accessToken);
            originalRequest.headers.set('Authorization', `Bearer ${accessToken}`);

            return apiClient(originalRequest);
        } catch (refreshError) {
            useAuthStore(pinia).clearSession();
            return Promise.reject(refreshError);
        }
    },
);