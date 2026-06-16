import { apiFetch } from '@/app-ui/Fetch/apiFetch';
import { setAccessToken } from '@/app-ui/Fetch/accessToken';
import type { LoginResponse } from '@/app-ui/Auth/types/LoginResponse';
import type { AuthUser } from '@/app-ui/Auth/types/AuthUser';
import { clearAuth, isAuthenticated, scheduleRefresh, scheduleRetry, user } from '@/app-ui/Auth/state';
import { clearSessionHint } from '@/app-ui/Auth/sessionHint';

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

// Transient-failure retry budget. A momentary 5xx / offline blip during a mid-
// session refresh must NOT log the user out (the access token is typically still
// valid — the rotation fires 30 s early): keep the session and retry with capped,
// jittered exponential backoff so it self-heals. The jitter de-synchronizes
// retries across tabs hit by the same outage — without it, two tabs would retry
// on identical marks and their refreshes would land together when the backend
// recovers, tripping the backend's concurrent-rotation theft detection (the
// multi-tab trade-off documented in the roadmap). After the budget is spent (or
// for a cold bootstrap that was never signed in) we give up: tear down in-memory
// state but KEEP the hint so a later page load can still restore.
const MAX_RETRIES = 5;
const RETRY_BASE_MS = 2_000;
let retries = 0;

// Single-flight guard. Bootstrap, the auto-refresh timer, its retries, and
// authFetch's 401 retry all share ONE in-flight rotation. Without this they
// could rotate the same cookie concurrently within a tab — which the backend's
// compare-and-swap treats as token theft and force-logs-out the session. This is
// therefore a security property; see the coalescing test.
let inFlight: Promise<boolean> | null = null;

// Exchanges the refresh cookie for a new access token + rotated refresh cookie.
// Never throws. A `true` result means a fresh session; a `false` result means
// EITHER "the session ended" OR "a transient miss is being retried in the
// background" — callers tolerate both (authFetch returns the original response,
// the timer voids it, bootstrap stays on the guest path).
//
// The gk_session hint is dropped ONLY on a definitive 401 (token invalid /
// revoked / expired — the server already cleared the cookie). A transient 5xx,
// a network/offline error, or a malformed body leaves the hint intact so a later
// load can still self-heal. Backend theft detection rides the used_at marker —
// see guides/auth.
export const refresh = (): Promise<boolean> => {
    if (inFlight !== null) {
        return inFlight;
    }
    inFlight = runRefresh().finally(() => {
        inFlight = null;
    });

    return inFlight;
};

const runRefresh = async (): Promise<boolean> => {
    try {
        const result = await apiFetch<LoginResponse>('POST', '/api/v1/auth/refresh');

        if (result.success === true) {
            // A 200 with a malformed/partial body is not a usable session — treat
            // it as a transient miss (it may be a flaky proxy), not a hard logout.
            if (isLoginResponse(result.data) === false) {
                return onTransientFailure();
            }

            retries = 0;
            setAccessToken(result.data.access_token);
            user.value = result.data.user;
            isAuthenticated.value = true;
            scheduleRefresh(result.data.access_expiration * 1_000, () => {
                void refresh();
            });

            return true;
        }

        // Definitive auth failure → the stored session is invalid; drop the hint
        // so the bootstrap stops retrying it, and tear down.
        if (result.status === 401) {
            retries = 0;
            clearSessionHint();
            clearAuth();

            return false;
        }

        // Any other status (5xx) is transient.
        return onTransientFailure();
    } catch {
        // Network/transport error — transient.
        return onTransientFailure();
    }
};

// A transient miss must not end a live session. Mid-session (authenticated) and
// within budget → keep the session + timer and retry after a jittered backoff.
// Otherwise (cold bootstrap, or budget spent) → tear down but KEEP the hint so a
// later load can retry.
const onTransientFailure = (): boolean => {
    if (isAuthenticated.value === true && retries < MAX_RETRIES) {
        retries += 1;
        const base = RETRY_BASE_MS * 2 ** (retries - 1); // 2s, 4s, 8s, 16s, 32s
        const jittered = base * (0.5 + Math.random()); // ±50% — de-sync tabs

        scheduleRetry(jittered, () => {
            void refresh();
        });

        return false;
    }

    retries = 0;
    clearAuth();

    return false;
};
