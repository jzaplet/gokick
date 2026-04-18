import { apiFetch } from '@/app-ui/Fetch/apiFetch';
import { setAccessToken } from '@/app-ui/Fetch/accessToken';
import type { LoginResponse } from '@/app-ui/Auth/types/LoginResponse';
import { clearAuth, isAuthenticated, scheduleRefresh, user } from '@/app-ui/Auth/state';

// Exchanges the refresh cookie for a new access token + rotated refresh cookie.
// Returns true on success, false otherwise (state is cleared on failure).
// The backend handles theft detection (used_at marker) — see guides/auth.
export const refresh = async (): Promise<boolean> => {
    const result = await apiFetch<LoginResponse>('POST', '/api/v1/auth/refresh');

    if (result.success === true) {
        setAccessToken(result.data.access_token);
        user.value = result.data.user;
        isAuthenticated.value = true;
        scheduleRefresh(result.data.access_expiration * 1_000, () => {
            void refresh();
        });

        return true;
    }

    clearAuth();

    return false;
};
