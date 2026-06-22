import { user } from '@/app-ui/Auth/state';
import type { Permission } from '@/app/Auth/enums/resources';

// Mirrors backend IsPermissionAllowedForRole (app/domain/shared/permission.go)
// byte-for-byte: superadmin → everything; admin → everything except platform:*;
// everyone else relies on the server-supplied user.permissions list.

export const hasRole = (role: string): boolean => {
    return user.value?.role === role;
};

export const isAdmin = (): boolean => {
    return hasRole('admin');
};

export const hasPermission = (permission: Permission): boolean => {
    if (user.value === null) {
        return false;
    }

    if (user.value.role === 'superadmin') {
        return true;
    }

    // platform:* is the platform plane — superadmin only. An admin's
    // "everything below" must not swallow it (mirrors the backend gate order).
    if (permission.startsWith('platform:') === true) {
        return false;
    }

    if (user.value.role === 'admin') {
        return true;
    }

    return user.value.permissions.includes(permission);
};

export const hasAllPermissions = (permissions: Permission[]): boolean => {
    for (const permission of permissions) {
        if (hasPermission(permission) === false) {
            return false;
        }
    }

    return true;
};

export const hasAnyPermission = (permissions: Permission[]): boolean => {
    for (const permission of permissions) {
        if (hasPermission(permission) === true) {
            return true;
        }
    }

    return false;
};
