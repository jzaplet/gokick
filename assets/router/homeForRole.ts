import { Role } from '@/app/Auth/enums/roles';

// Where a signed-in user belongs: the post-login landing page and the target of
// Home's "Dashboard" button.
//
// Keyed by ROLE, never by permission. The role ladder grants a superadmin
// everything below it — including admin:dashboard:read — so "may you read the
// admin dashboard?" answers true for a superadmin and strands them on the admin
// plane. A role has exactly one home; a permission does not.
//
// Record<Role, string> is exhaustive: a new role in the Go union regenerates
// roles.ts and breaks this map at compile time rather than silently landing the
// new role on the fallback.
const roleHome: Record<Role, string> = {
    [Role.SuperAdmin]: '/platform/dashboard',
    [Role.Admin]: '/admin/dashboard',
    [Role.User]: '/user/dashboard',
};

export const homeForRole = (role: Role): string => roleHome[role];
