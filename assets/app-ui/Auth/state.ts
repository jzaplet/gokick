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

// Wipes every trace of a session — called on logout, refresh failure, and when
// the 401 retry path ultimately gives up. Deliberately does NOT drop the
// gk_session hint: clearAuth also runs on transient failures (a 5xx/offline
// refresh), and clearing the hint there would skip the bootstrap refresh on the
// next load — turning a momentary backend hiccup into a durable logout even
// though the refresh cookie is still valid. The hint is cleared only at the
// definitive end of a session: an explicit logout and a 401 from refresh (see
// logout.ts / refresh.ts).
export const clearAuth = (): void => {
    setAccessToken(null);
    user.value = null;
    isAuthenticated.value = false;
    clearRefreshTimer();
};
