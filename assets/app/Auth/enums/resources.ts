// Mirrors backend RequiredPermission() declarations across
// app/application/**/(command|query)/*.go. Keep both sides in sync — backend
// is authoritative.

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
} as const;

export type Permission = typeof Permission[keyof typeof Permission];
