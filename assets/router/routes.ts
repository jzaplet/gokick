import type { AppRoute } from '@/router/meta';
import { Permission } from '@/app/Auth/enums/resources';
import { SUPPORTED_LANGS } from '@/app-ui/I18n';
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

// Every route carries an optional language prefix: English is
// canonical at the bare path, other languages live under /cs/… etc. The
// authGuard keeps URLs canonical (strips /en, adds the prefix for a non-en
// locale) and syncs the locale from the prefix. An unsupported prefix
// (/de/…) simply fails the regex and lands in the not-found catch-all.
const langPrefix = `/:lang(${SUPPORTED_LANGS.join('|')})?`;

// Each route declares its auth posture explicitly (mirrors the backend
// Permissioned / SkipPermission rule). TypeScript rejects any entry without
// meta.requiresAuth — there is no implicit "public".
export const routes: AppRoute[] = [
    {
        path: langPrefix,
        name: 'home',
        component: HomeView,
        meta: { requiresAuth: false },
    },
    {
        path: `${langPrefix}/login`,
        name: 'login',
        component: LoginView,
        meta: { requiresAuth: false },
    },
    {
        path: `${langPrefix}/profile`,
        name: 'profile',
        component: ProfileView,
        meta: { requiresAuth: true },
    },
    {
        path: `${langPrefix}/user/dashboard`,
        name: 'user-dashboard',
        component: UserDashboardView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.DashboardRead,
        },
    },
    {
        path: `${langPrefix}/admin/dashboard`,
        name: 'admin-dashboard',
        component: AdminDashboardView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.AdminDashboardRead,
        },
    },
    {
        path: `${langPrefix}/admin/users`,
        name: 'admin-users',
        component: AdminUsersView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.AdminUsersRead,
        },
    },
    {
        path: `${langPrefix}/admin/users/new`,
        name: 'admin-users-new',
        component: AdminUserCreateView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.AdminUsersCreate,
        },
    },
    {
        path: `${langPrefix}/admin/users/:id/edit`,
        name: 'admin-users-edit',
        component: AdminUserEditView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.AdminUsersUpdate,
        },
    },
    {
        path: `${langPrefix}/platform/dashboard`,
        name: 'platform-dashboard',
        component: PlatformDashboardView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.PlatformOverview,
        },
    },
    {
        path: `${langPrefix}/platform/tenants`,
        name: 'platform-tenants',
        component: PlatformTenantsView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.PlatformOverview,
        },
    },
    {
        path: `${langPrefix}/platform/tenants/new`,
        name: 'platform-tenants-new',
        component: PlatformTenantCreateView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.PlatformTenantsCreate,
        },
    },
    {
        path: `${langPrefix}/platform/users`,
        name: 'platform-users',
        component: PlatformUsersView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.PlatformOverview,
        },
    },
    {
        path: `${langPrefix}/platform/users/new`,
        name: 'platform-users-new',
        component: PlatformUserCreateView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.PlatformUsersCreate,
        },
    },
    {
        path: `${langPrefix}/platform/users/:id/edit`,
        name: 'platform-users-edit',
        component: PlatformUserEditView,
        meta: {
            requiresAuth: true,
            requiresPermission: Permission.PlatformUsersUpdate,
        },
    },
    {
        // Trailing catch-all: an unmatched path renders the 404 view instead of a
        // blank RouterView. Carries the language prefix too, so /cs/nonsense is a
        // Czech 404 and the guard's prefix logic applies uniformly (no redirect
        // loop on prefix-less records). requiresAuth:false so both signed-in and
        // signed-out users see it.
        path: `${langPrefix}/:pathMatch(.*)*`,
        name: 'not-found',
        component: NotFoundView,
        meta: { requiresAuth: false },
    },
];
