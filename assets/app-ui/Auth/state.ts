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

// Arms the single auto-refresh/retry timer to fire fn after delayMs (floored at
// 1 s). Replaces any pending timer, so there is never more than one in flight.
// A non-finite delay (NaN/Infinity from a malformed expiration) arms nothing —
// otherwise setTimeout would fire immediately and spin a hot loop. refresh.ts
// shape-guards the body before scheduling; this is the backstop.
const armRefreshTimer = (delayMs: number, fn: () => void): void => {
    clearRefreshTimer();
    if (Number.isFinite(delayMs) === false) {
        return;
    }
    refreshTimer = setTimeout(fn, Math.max(delayMs, 1_000));
};

// Schedules the refresh call 30 s before the access token expires.
// Callers pass their own refresh function to avoid a circular import.
export const scheduleRefresh = (expiresInMs: number, fn: () => void): void => {
    armRefreshTimer(expiresInMs - 30_000, fn);
};

// Schedules a retry after a transient refresh failure (the caller computes the
// backoff). Shares the single timer with scheduleRefresh, so clearAuth/logout
// cancels a pending retry and a later success re-arms the normal rotation in its
// place — there is never a refresh AND a retry timer alive at once.
export const scheduleRetry = (delayMs: number, fn: () => void): void => {
    armRefreshTimer(delayMs, fn);
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
