import { user } from '@/app-ui/Auth/state';
import type { Permission } from '@/app/Auth/enums/resources';
import { Role } from '@/app/Auth/enums/roles';

// Mirrors backend IsPermissionAllowedForRole (app/domain/shared/permission.go)
// byte-for-byte: superadmin → everything; admin → everything except platform:*;
// everyone else relies on the server-supplied user.permissions list.

export const hasRole = (role: Role): boolean => {
    return user.value?.role === role;
};

export const isAdmin = (): boolean => {
    return hasRole(Role.Admin);
};

export const isSuperAdmin = (): boolean => {
    return hasRole(Role.SuperAdmin);
};

export const hasPermission = (permission: Permission): boolean => {
    // The server ships the authoritative, role-filtered permission list
    // (PermissionsRegistry.ForRole → user.permissions on every login/refresh), so
    // the FE does a UNIFORM membership check for every role instead of
    // re-implementing the backend role ladder here (which silently drifted from
    // IsPermissionAllowedForRole — admin/superadmin ignored the list entirely).
    // List completeness is enforced by the di registry-conformance gate (F-040).
    if (user.value === null) {
        return false;
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
