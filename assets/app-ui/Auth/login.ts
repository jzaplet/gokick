import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';
import { apiFetch } from '@/app-ui/Fetch/apiFetch';
import type { AuthError } from '@/app-ui/Auth/types/AuthError';
import type { LoginRequest } from '@/app-ui/Auth/types/LoginRequest';
import type { LoginResponse } from '@/app-ui/Auth/types/LoginResponse';
import { establishSession } from '@/app-ui/Auth/state';
import { refresh } from '@/app-ui/Auth/refresh';

// POST /api/v1/auth/login — generic TError lets callers supply their own
// error shape (e.g. `{ general?; nickname?; password? }` in LoginForm).
// No default: the caller must declare the error shape it wants to handle.
export const login = async <TError extends AuthError>(
    credentials: LoginRequest,
): Promise<ApiResponse<LoginResponse, TError>> => {
    const result = await apiFetch<LoginResponse, TError, LoginRequest>('POST', '/api/v1/auth/login', {
        body: credentials,
    });

    // establishSession shape-guards the 200 body before installing the session, so
    // a malformed body can't flip isAuthenticated on a bogus/empty access token —
    // the same guard refresh already had (previously login trusted the body blindly).
    if (result.success === true) {
        establishSession(result.data, () => void refresh());
    }

    return result;
};
