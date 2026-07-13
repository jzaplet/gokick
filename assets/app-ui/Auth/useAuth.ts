import { type DeepReadonly, readonly } from 'vue';
import { isAuthenticated, user } from '@/app-ui/Auth/state';
import { login } from '@/app-ui/Auth/login';
import { logout } from '@/app-ui/Auth/logout';
import { refresh } from '@/app-ui/Auth/refresh';
import {
    hasAllPermissions,
    hasAnyPermission,
    hasPermission,
    hasRole,
    isAdmin,
    isSuperAdmin,
} from '@/app-ui/Auth/permissions';

// Thin composable — exposes the session state as readonly refs and the
// action functions as-is. Each concern lives in its own file (state, login,
// logout, refresh, permissions); useAuth is just the orchestrator.
export const useAuth = (): {
    user: DeepReadonly<typeof user>;
    isAuthenticated: DeepReadonly<typeof isAuthenticated>;
    login: typeof login;
    logout: typeof logout;
    refresh: typeof refresh;
    hasRole: typeof hasRole;
    isAdmin: typeof isAdmin;
    isSuperAdmin: typeof isSuperAdmin;
    hasPermission: typeof hasPermission;
    hasAllPermissions: typeof hasAllPermissions;
    hasAnyPermission: typeof hasAnyPermission;
} => {
    return {
        user: readonly(user),
        isAuthenticated: readonly(isAuthenticated),
        login,
        logout,
        refresh,
        hasRole,
        isAdmin,
        isSuperAdmin,
        hasPermission,
        hasAllPermissions,
        hasAnyPermission,
    };
};
