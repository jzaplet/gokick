import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';
import { apiFetch } from '@/app-ui/Fetch/apiFetch';
import { setAccessToken } from '@/app-ui/Fetch/accessToken';
import type { AuthError } from '@/app-ui/Auth/types/AuthError';
import type { LoginRequest } from '@/app-ui/Auth/types/LoginRequest';
import type { LoginResponse } from '@/app-ui/Auth/types/LoginResponse';
import { isAuthenticated, scheduleRefresh, user } from '@/app-ui/Auth/state';
import { refresh } from '@/app-ui/Auth/refresh';

// POST /api/v1/auth/login — returns the full ApiResponse so the caller
// (e.g. LoginView) can render backend validation messages on 401.
export const login = async (
    credentials: LoginRequest,
): Promise<ApiResponse<LoginResponse, AuthError>> => {
    const result = await apiFetch<LoginResponse>('POST', '/api/v1/auth/login', {
        body: credentials,
    });

    if (result.success === true) {
        setAccessToken(result.data.access_token);
        user.value = result.data.user;
        isAuthenticated.value = true;
        scheduleRefresh(result.data.access_expiration * 1_000, () => {
            void refresh();
        });
    }

    return result;
};
