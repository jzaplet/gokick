import { apiFetch } from '@/app-ui/Fetch/apiFetch';
import { setAccessToken } from '@/app-ui/Fetch/accessToken';
import type { LoginResponse } from '@/app-ui/Auth/types/LoginResponse';
import type { AuthUser } from '@/app-ui/Auth/types/AuthUser';
import { clearAuth, isAuthenticated, scheduleRefresh, user } from '@/app-ui/Auth/state';
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

// Exchanges the refresh cookie for a new access token + rotated refresh cookie.
// Returns true on success, false otherwise. Never throws — a transport failure
// OR a malformed success body that throws while we read it is treated as a
// transient miss, so callers (bootstrap, the auto-refresh timer, authFetch's 401
// retry) don't each need their own try/catch. The whole body, including the
// success-path field access, is inside the try for exactly that reason.
//
// The gk_session hint is dropped ONLY on a definitive 401 (the token is
// invalid/revoked/expired — the server has already cleared the cookie). A
// transient 5xx, a network/offline error, or a thrown malformed body leaves the
// hint intact so the next page load can still self-heal instead of being stuck
// logged out. The backend handles theft detection (used_at marker) — see
// guides/auth.
export const refresh = async (): Promise<boolean> => {
    try {
        const result = await apiFetch<LoginResponse>('POST', '/api/v1/auth/refresh');

        if (result.success === true) {
            // A 200 with a malformed/partial body is not a usable session. Treat
            // it as a transient miss: tear down in-memory state but KEEP the hint
            // (clearAuth, not clearSessionHint) so a later load can retry.
            if (isLoginResponse(result.data) === false) {
                clearAuth();

                return false;
            }

            setAccessToken(result.data.access_token);
            user.value = result.data.user;
            isAuthenticated.value = true;
            scheduleRefresh(result.data.access_expiration * 1_000, () => {
                void refresh();
            });

            return true;
        }

        // Definitive auth failure → the stored session is invalid; drop the hint
        // so the bootstrap stops retrying it. Any other status (5xx) is transient
        // and leaves the hint alone.
        if (result.status === 401) {
            clearSessionHint();
        }

        clearAuth();

        return false;
    } catch {
        // Network/transport error, or a malformed success body that threw while
        // being read — both transient. Tear down in-memory state but KEEP the
        // hint so a later load can retry the restore.
        clearAuth();

        return false;
    }
};
