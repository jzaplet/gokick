import type { AppRoute } from '@/router/meta';
import { Permission } from '@/app/Auth/enums/resources';
import HomeView from '@/app/Home/Views/HomeView.vue';
import NotFoundView from '@/app/Home/Views/NotFoundView.vue';
import LoginView from '@/app/Auth/Views/LoginView.vue';
import ProfileView from '@/app/Profile/Views/ProfileView.vue';
import AdminUsersView from '@/app/Admin/Views/AdminUsersView.vue';
import AdminUserCreateView from '@/app/Admin/Views/AdminUserCreateView.vue';
import AdminUserEditView from '@/app/Admin/Views/AdminUserEditView.vue';
import UserDashboardView from '@/app/Dashboard/Views/UserDashboardView.vue';
import AdminDashboardView from '@/app/Dashboard/Views/AdminDashboardView.vue';
import PlatformDashboardView from '@/app/Platform/Views/PlatformDashboardView.vue';
import PlatformTenantsView from '@/app/Platform/Views/PlatformTenantsView.vue';
import PlatformTenantCreateView from '@/app/Platform/Views/PlatformTenantCreateView.vue';
import PlatformUsersView from '@/app/Platform/Views/PlatformUsersView.vue';
import PlatformUserCreateView from '@/app/Platform/Views/PlatformUserCreateView.vue';
import PlatformUserEditView from '@/app/Platform/Views/PlatformUserEditView.vue';

// Each route declares its auth posture explicitly (mirrors the backend
// Permissioned / SkipPermission rule). TypeScript rejects any entry without
// meta.requiresAuth — there is no implicit "public".
export const routes: AppRoute[] = [
    {
        path: '/',
        name: 'home',
        component: HomeView,
        meta: { requiresAuth: false },
    },
    {
        path: '/login',
        name: 'login',
        component: LoginView,
        meta: { requiresAuth: false },
    },
    {
        path: '/profile',
        name: 'profile',
        component: ProfileView,
        meta: { requiresAuth: true },
    },
    {
        path: '/user/dashboard',
        name: 'user-dashboard',
        component: UserDashboardView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.DashboardRead,
        },
    },
    {
        path: '/admin/dashboard',
        name: 'admin-dashboard',
        component: AdminDashboardView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.AdminDashboardRead,
        },
    },
    {
        path: '/admin/users',
        name: 'admin-users',
        component: AdminUsersView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.AdminUsersRead,
        },
    },
    {
        path: '/admin/users/new',
        name: 'admin-users-new',
        component: AdminUserCreateView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.AdminUsersCreate,
        },
    },
    {
        path: '/admin/users/:id/edit',
        name: 'admin-users-edit',
        component: AdminUserEditView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.AdminUsersUpdate,
        },
    },
    {
        path: '/platform/dashboard',
        name: 'platform-dashboard',
        component: PlatformDashboardView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.PlatformOverview,
        },
    },
    {
        path: '/platform/tenants',
        name: 'platform-tenants',
        component: PlatformTenantsView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.PlatformOverview,
        },
    },
    {
        path: '/platform/tenants/new',
        name: 'platform-tenants-new',
        component: PlatformTenantCreateView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.PlatformTenantsCreate,
        },
    },
    {
        path: '/platform/users',
        name: 'platform-users',
        component: PlatformUsersView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.PlatformOverview,
        },
    },
    {
        path: '/platform/users/new',
        name: 'platform-users-new',
        component: PlatformUserCreateView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.PlatformUsersCreate,
        },
    },
    {
        path: '/platform/users/:id/edit',
        name: 'platform-users-edit',
        component: PlatformUserEditView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.PlatformUsersUpdate,
        },
    },
    {
        // Trailing catch-all: an unmatched path renders the 404 view instead of a
        // blank RouterView. requiresAuth:false so both signed-in and signed-out
        // users see it (authGuard passes requiresAuth:false straight through).
        path: '/:pathMatch(.*)*',
        name: 'not-found',
        component: NotFoundView,
        meta: { requiresAuth: false },
    },
];
