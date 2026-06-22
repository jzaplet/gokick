import { describe, expect, it, beforeEach } from 'vitest';
import { clearAuth, user } from '@/app-ui/Auth';
import { hasPermission } from '@/app-ui/Auth/permissions';
import { Permission } from '@/app/Auth/enums/resources';

// Mirrors the backend IsPermissionAllowedForRole ladder. The critical assertion
// is that an admin is NOT granted platform:* — the same boundary the Go
// permission tests pin, kept honest on the frontend.

const setLoggedIn = (role: string, permissions: string[] = []): void => {
    user.value = {
        id: 'u-1',
        nickname: 'alice',
        email: '',
        role,
        permissions,
    };
};

describe('hasPermission', () => {
    beforeEach((): void => {
        clearAuth();
    });

    it('returns false when not logged in', (): void => {
        expect(hasPermission(Permission.ProfileRead)).toBe(false);
    });

    it('superadmin gets the platform plane and everything below', (): void => {
        setLoggedIn('superadmin');
        expect(hasPermission(Permission.PlatformOverview)).toBe(true);
        expect(hasPermission(Permission.AdminUsersDelete)).toBe(true);
        expect(hasPermission(Permission.ProfileRead)).toBe(true);
    });

    it('admin gets admin:* and below but NOT the platform plane', (): void => {
        setLoggedIn('admin');
        expect(hasPermission(Permission.AdminUsersRead)).toBe(true);
        expect(hasPermission(Permission.PlatformOverview)).toBe(false);
    });

    it('user is denied admin and platform, allowed listed permissions', (): void => {
        setLoggedIn('user', ['profile:read']);
        expect(hasPermission(Permission.PlatformOverview)).toBe(false);
        expect(hasPermission(Permission.AdminUsersRead)).toBe(false);
        expect(hasPermission(Permission.ProfileRead)).toBe(true);
    });
});
