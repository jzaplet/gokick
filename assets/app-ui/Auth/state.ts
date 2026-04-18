import { ref } from 'vue';
import type { AuthUser } from '@/app-ui/Auth/types/AuthUser';
import { setAccessToken } from '@/app-ui/Fetch/accessToken';

// Reactive session state — single source of truth for views.
export const user = ref<AuthUser | null>(null);
export const isAuthenticated = ref(false);

// Auto-refresh timer — only one pending refresh at a time.
let refreshTimer: ReturnType<typeof setTimeout> | null = null;

const clearRefreshTimer = (): void => {
    if (refreshTimer !== null) {
        clearTimeout(refreshTimer);
        refreshTimer = null;
    }
};

// Schedules the refresh call 30 s before the access token expires.
// Callers pass their own refresh function to avoid a circular import.
export const scheduleRefresh = (expiresInMs: number, fn: () => void): void => {
    clearRefreshTimer();
    const delay = Math.max(expiresInMs - 30_000, 1_000);

    refreshTimer = setTimeout(fn, delay);
};

// Wipes every trace of a session — called on logout, refresh failure, and
// when the 401 retry path ultimately gives up.
export const clearAuth = (): void => {
    setAccessToken(null);
    user.value = null;
    isAuthenticated.value = false;
    clearRefreshTimer();
};
