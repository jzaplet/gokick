import { apiFetch } from '@/app-ui/Fetch/apiFetch';
import { setAccessToken } from '@/app-ui/Fetch/accessToken';
import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';
import type { LoginResponse } from '@/app-ui/Auth/types/LoginResponse';
import { clearAuth, isAuthenticated, scheduleRefresh, user } from '@/app-ui/Auth/state';
import { clearSessionHint } from '@/app-ui/Auth/sessionHint';

// Exchanges the refresh cookie for a new access token + rotated refresh cookie.
// Returns true on success, false otherwise. Never throws — a transport failure
// is treated as a transient miss, so callers (bootstrap, the auto-refresh timer,
// authFetch's 401 retry) don't each need their own try/catch.
//
// The gk_session hint is dropped ONLY on a definitive 401 (the token is
// invalid/revoked/expired — the server has already cleared the cookie). A
// transient 5xx, or a network/offline error, leaves the hint intact so the next
// page load can still self-heal instead of being stuck logged out. The backend
// handles theft detection (used_at marker) — see guides/auth.
export const refresh = async (): Promise<boolean> => {
    let result: ApiResponse<LoginResponse, { message: string }>;

    try {
        result = await apiFetch<LoginResponse>('POST', '/api/v1/auth/refresh');
    } catch {
        // Network/transport error — transient. Tear down in-memory state but
        // KEEP the hint so a later load can retry the restore.
        clearAuth();

        return false;
    }

    if (result.success === true) {
        setAccessToken(result.data.access_token);
        user.value = result.data.user;
        isAuthenticated.value = true;
        scheduleRefresh(result.data.access_expiration * 1_000, () => {
            void refresh();
        });

        return true;
    }

    // Definitive auth failure → the stored session is invalid; drop the hint so
    // the bootstrap stops retrying it. Any other status (5xx) is transient and
    // leaves the hint alone.
    if (result.status === 401) {
        clearSessionHint();
    }

    clearAuth();

    return false;
};
