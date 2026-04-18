import type { AppRoute } from '@/router/meta';
import HomeView from '@/app/Home/Views/HomeView.vue';
import LoginView from '@/app/Auth/Views/LoginView.vue';
import ProfileView from '@/app/Profile/Views/ProfileView.vue';
import AdminUsersView from '@/app/Admin/Views/AdminUsersView.vue';

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
        path: '/admin/users',
        name: 'admin-users',
        component: AdminUsersView,
        meta: {
            requiresAuth: true,
            requiresPermission: 'admin:users:read',
        },
    },
];
