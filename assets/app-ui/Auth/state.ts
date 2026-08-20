import { ref } from 'vue';
import type { AuthUser } from '@/app-ui/Auth/types/AuthUser';
import { isLoginResponse } from '@/app-ui/Auth/types/LoginResponse';
import { setAccessToken } from '@/app-ui/Fetch/accessToken';
import { adoptServerLang } from '@/app-ui/I18n';

// Reactive session state — single source of truth for views.
export const user = ref<AuthUser | null>(null);
export const isAuthenticated = ref(false);

// Auto-refresh timer — only one pending refresh at a time.
let refreshTimer: ReturnType<typeof setTimeout> | null = null;

// Whether THIS authenticated session already adopted the profile language.
// establishSession also runs on every ~15-min background token rotation, and
// re-adopting there would flip a cookie-less user's URL-chosen locale
// mid-session. Reset in clearAuth so the NEXT login adopts again.
let langAdopted = false;

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

// establishSession validates a login/refresh 200 body and, if it is a usable
// session, installs it (access token, user, isAuthenticated, scheduled rotation)
// and returns true. A malformed/partial 200 (empty access_token, missing/void
// fields) installs NOTHING and returns false — so neither login nor refresh can
// flip isAuthenticated on a bogus body (the asymmetry that let login trust an
// unvalidated body). onExpiry is the caller's refresh trigger, passed in to avoid
// the state <-> refresh circular import.
export const establishSession = (data: unknown, onExpiry: () => void): boolean => {
    // Structure comes from the GENERATED guard (the same one login/refresh pass
    // to the fetch layer — this re-check keeps the seam safe when called
    // directly, e.g. from tests). The extra checks are SEMANTIC: an empty token
    // or a non-finite expiration is structurally valid but not a usable session
    // (setAccessToken('') + scheduleRefresh(NaN) would spin a hot refresh loop).
    // The role needs no check here: since it is generated as a union, isRole
    // inside the guard already rejects '' — the codegen absorbed what used to
    // be a hand-written semantic check.
    if (
        isLoginResponse(data) === false
        || data.access_token === ''
        || Number.isFinite(data.access_expiration) === false
    ) {
        return false;
    }
    setAccessToken(data.access_token);
    user.value = data.user;
    isAuthenticated.value = true;
    // Adopt the persisted profile language ("" = none) — unless this browser
    // chose explicitly via the switcher. Only the session's FIRST establish
    // (bootstrap refresh or interactive login) adopts; background rotations
    // must not re-flip the locale.
    if (langAdopted === false) {
        adoptServerLang(data.user.lang);
        langAdopted = true;
    }
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
    langAdopted = false;
};
