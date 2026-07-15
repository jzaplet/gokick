import { describe, expect, it, beforeEach } from 'vitest';
import { clearAuth, user } from '@/app-ui/Auth';
import { hasPermission } from '@/app-ui/Auth/permissions';
import { Permission } from '@/app/Auth/enums/resources';

// hasPermission is now a uniform membership check over the server-supplied,
// role-filtered permission list (registry.ForRole ships it on every login/refresh)
// — no FE role ladder. So each test seeds the list the SERVER actually sends for
// that role. The critical boundary (an admin is NOT granted platform:*) is pinned
// by the admin list simply not containing it — the Go side guarantees that.

const setLoggedIn = (role: string, permissions: string[]): void => {
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

    it('superadmin gets the platform plane and everything below (server sends the full list)', (): void => {
        setLoggedIn('superadmin', ['platform:overview', 'admin:users:delete', 'profile:read']);
        expect(hasPermission(Permission.PlatformOverview)).toBe(true);
        expect(hasPermission(Permission.AdminUsersDelete)).toBe(true);
        expect(hasPermission(Permission.ProfileRead)).toBe(true);
    });

    it('admin gets admin:* and below but NOT the platform plane (server omits platform:*)', (): void => {
        setLoggedIn('admin', ['admin:users:read', 'profile:read']);
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
