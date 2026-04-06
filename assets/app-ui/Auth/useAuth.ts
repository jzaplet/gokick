import { readonly, ref } from 'vue';
import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';
import { apiFetch, setAccessToken } from '@/app-ui/Fetch/useFetch';
import type { AuthUser } from '@/app-ui/Auth/types/AuthUser';
import type { AuthError } from '@/app-ui/Auth/types/AuthError';
import type { LoginRequest } from '@/app-ui/Auth/types/LoginRequest';
import type { LoginResponse } from '@/app-ui/Auth/types/LoginResponse';

const user = ref<AuthUser | null>(null);
const isAuthenticated = ref(false);

let refreshTimer: ReturnType<typeof setTimeout> | null = null;

const scheduleRefresh = (expiresInMs: number): void => {
    if (refreshTimer !== null) {
        clearTimeout(refreshTimer);
    }

    // Refresh 30 seconds before expiration
    const delay = Math.max(expiresInMs - 30_000, 1_000);

    refreshTimer = setTimeout(() => {
        void refresh();
    }, delay);
};

const clearAuth = (): void => {
    setAccessToken(null);
    user.value = null;
    isAuthenticated.value = false;

    if (refreshTimer !== null) {
        clearTimeout(refreshTimer);
        refreshTimer = null;
    }
};

// --- Auth actions ---

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
        scheduleRefresh(result.data.access_expiration * 1_000);
    }

    return result;
};

export const refresh = async (): Promise<boolean> => {
    const result = await apiFetch<LoginResponse>('POST', '/api/v1/auth/refresh');

    if (result.success === true) {
        setAccessToken(result.data.access_token);
        user.value = result.data.user;
        isAuthenticated.value = true;
        scheduleRefresh(result.data.access_expiration * 1_000);

        return true;
    }

    clearAuth();

    return false;
};

export const logout = async (): Promise<void> => {
    await apiFetch<unknown>('POST', '/api/v1/auth/logout');

    clearAuth();
};

// --- Role & permission helpers ---

export const hasRole = (role: string): boolean => {
    return user.value?.role === role;
};

export const isAdmin = (): boolean => {
    return hasRole('admin');
};

export const hasPermission = (permission: string): boolean => {
    if (user.value === null) {
        return false;
    }

    if (user.value.role === 'admin') {
        return true;
    }

    return user.value.permissions.includes(permission);
};

export const hasAllPermissions = (permissions: string[]): boolean => {
    for (const permission of permissions) {
        if (hasPermission(permission) === false) {
            return false;
        }
    }

    return true;
};

export const hasAnyPermission = (permissions: string[]): boolean => {
    for (const permission of permissions) {
        if (hasPermission(permission) === true) {
            return true;
        }
    }

    return false;
};

// --- Composable ---

export const useAuth = (): {
    user: typeof user;
    isAuthenticated: typeof isAuthenticated;
    login: typeof login;
    logout: typeof logout;
    refresh: typeof refresh;
    hasRole: typeof hasRole;
    isAdmin: typeof isAdmin;
    hasPermission: typeof hasPermission;
    hasAllPermissions: typeof hasAllPermissions;
    hasAnyPermission: typeof hasAnyPermission;
} => {
    return {
        user: readonly(user) as typeof user,
        isAuthenticated: readonly(isAuthenticated) as typeof isAuthenticated,
        login,
        logout,
        refresh,
        hasRole,
        isAdmin,
        hasPermission,
        hasAllPermissions,
        hasAnyPermission,
    };
};
