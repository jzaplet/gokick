import { ref } from 'vue';
import type { AuthUser } from '@/app-ui/Auth/types/AuthUser';
import type { LoginResponse } from '@/app-ui/Auth/types/LoginResponse';
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

// Validates the user principal carries the fields the app later dereferences —
// notably role (string) and permissions (string[]). Checking only `typeof user
// === 'object'` is NOT enough: `user:{}` or `user:[]` (typeof [] === 'object')
// would pass, isAuthenticated would flip true, then the router guard would crash
// at `user.permissions.includes(...)` (permissions.ts). Narrows from unknown so
// every field access below is a real runtime check, not an erased cast.
const isAuthUser = (data: unknown): data is AuthUser =>
    typeof data === 'object'
    && data !== null
    && Array.isArray(data) === false
    && 'id' in data
    && typeof data.id === 'string'
    && 'nickname' in data
    && typeof data.nickname === 'string'
    && 'email' in data
    && typeof data.email === 'string'
    && 'role' in data
    && typeof data.role === 'string'
    && data.role !== ''
    && 'permissions' in data
    && Array.isArray(data.permissions) === true;

// parseResponse casts the body to LoginResponse, but a 200 with an empty or
// partial body yields null / a missing field at runtime. Guard the shape before
// trusting it: without this, setAccessToken(undefined) + scheduleRefresh(NaN)
// would spin a hot refresh loop. Narrows from unknown (no `as`) so the runtime
// checks are real rather than erased by the optimistic cast.
const isLoginResponse = (data: unknown): data is LoginResponse =>
    typeof data === 'object'
    && data !== null
    && 'access_token' in data
    && typeof data.access_token === 'string'
    && data.access_token !== ''
    && 'access_expiration' in data
    && typeof data.access_expiration === 'number'
    && Number.isFinite(data.access_expiration) === true
    && 'user' in data
    && isAuthUser(data.user);

// establishSession validates a login/refresh 200 body and, if it is a usable
// session, installs it (access token, user, isAuthenticated, scheduled rotation)
// and returns true. A malformed/partial 200 (empty access_token, missing/void
// fields) installs NOTHING and returns false — so neither login nor refresh can
// flip isAuthenticated on a bogus body (the asymmetry that let login trust an
// unvalidated body). onExpiry is the caller's refresh trigger, passed in to avoid
// the state <-> refresh circular import.
export const establishSession = (data: unknown, onExpiry: () => void): boolean => {
    if (isLoginResponse(data) === false) {
        return false;
    }
    setAccessToken(data.access_token);
    user.value = data.user;
    isAuthenticated.value = true;
    scheduleRefresh(data.access_expiration * 1_000, onExpiry);

    return true;
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
