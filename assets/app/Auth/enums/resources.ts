// Mirrors the FE-reachable backend permissions — those the PermissionsRegistry
// surfaces to the frontend (login / profile responses). CLI-only permissions
// (shared.CLIOnly: create-superadmin, get-tenant) are deliberately excluded — the
// frontend can never invoke them. Kept in sync by hand for now; backend is
// authoritative. Automating this mirror is the BE↔FE codegen follow-up (see
// gokick-roadmap).
//
// create-tenant is NOT in that CLI-only list any more: the superadmin plane now
// offers it over HTTP, so platform:tenants:create is FE-reachable and appears
// below. Minting a superadmin stays CLI-only and now carries its own
// platform:superadmins:create, which is why no entry here names it.

export const Permission = {
    AuthLogout: 'auth:logout',
    DashboardRead: 'dashboard:read',
    ProfileRead: 'profile:read',
    ProfileUpdate: 'profile:update',
    AdminDashboardRead: 'admin:dashboard:read',
    AdminUsersRead: 'admin:users:read',
    AdminUsersCreate: 'admin:users:create',
    AdminUsersUpdate: 'admin:users:update',
    AdminUsersDelete: 'admin:users:delete',
    PlatformOverview: 'platform:overview',
    PlatformUsersCreate: 'platform:users:create',
    PlatformUsersUpdate: 'platform:users:update',
    PlatformUsersDelete: 'platform:users:delete',
    PlatformTenantsCreate: 'platform:tenants:create',
    PlatformTenantsDelete: 'platform:tenants:delete',
} as const;

export type Permission = typeof Permission[keyof typeof Permission];
