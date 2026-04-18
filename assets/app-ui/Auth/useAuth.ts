import { readonly } from 'vue';
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
} from '@/app-ui/Auth/permissions';

// Thin composable — exposes the session state as readonly refs and the
// action functions as-is. Each concern lives in its own file (state, login,
// logout, refresh, permissions); useAuth is just the orchestrator.
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
