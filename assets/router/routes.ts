import type { AppRoute } from '@/router/meta';
import { Permission } from '@/app/Auth/enums/resources';
import HomeView from '@/app/Home/Views/HomeView.vue';
import LoginView from '@/app/Auth/Views/LoginView.vue';
import ProfileView from '@/app/Profile/Views/ProfileView.vue';
import AdminUsersView from '@/app/Admin/Views/AdminUsersView.vue';
import AdminUserCreateView from '@/app/Admin/Views/AdminUserCreateView.vue';
import AdminUserEditView from '@/app/Admin/Views/AdminUserEditView.vue';
import UserDashboardView from '@/app/Dashboard/Views/UserDashboardView.vue';
import AdminDashboardView from '@/app/Dashboard/Views/AdminDashboardView.vue';

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
];
