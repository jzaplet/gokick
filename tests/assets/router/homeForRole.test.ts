import { describe, expect, it } from 'vitest';
import { Role } from '@/app/Auth/enums/roles';
import { homeForRole } from '@/router/homeForRole';

describe('homeForRole', () => {
    // The regression this file exists for: Home asked hasPermission(
    // 'admin:dashboard:read') and sent whoever answered true to /admin/dashboard.
    // A superadmin answers true — the ladder grants them every admin:* permission
    // — so the platform plane was unreachable from the Home button.
    it('sends a superadmin to the platform plane, not the admin plane', () => {
        expect(homeForRole(Role.SuperAdmin)).toBe('/platform/dashboard');
    });

    it('sends an admin to the admin plane', () => {
        expect(homeForRole(Role.Admin)).toBe('/admin/dashboard');
    });

    it('sends a plain user to the user plane', () => {
        expect(homeForRole(Role.User)).toBe('/user/dashboard');
    });

    // Every role resolves to a distinct home. Guards against a copy-paste that
    // points two roles at the same plane — which is what the bug looked like
    // from the outside (superadmin and admin sharing /admin/dashboard).
    it('gives every role its own home', () => {
        const homes = [Role.SuperAdmin, Role.Admin, Role.User].map(homeForRole);

        expect(new Set(homes).size).toBe(homes.length);
    });
});
