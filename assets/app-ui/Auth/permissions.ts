import { user } from '@/app-ui/Auth/state';
import type { Permission } from '@/app/Auth/enums/resources';

// Mirrors the backend rule: admin has everything, others rely on the
// server-supplied user.permissions list.

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
