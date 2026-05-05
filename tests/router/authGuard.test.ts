import { describe, expect, it, beforeEach } from 'vitest';
import { createMemoryHistory, createRouter } from 'vue-router';
import type { Router } from 'vue-router';
import { type AppRoute, authGuard } from '@/router';
import { clearAuth, isAuthenticated, user } from '@/app-ui/Auth';

const Blank = { template: '<div />' };

const makeTestRouter = (): Router => {
    const routes: AppRoute[] = [
        { path: '/', name: 'home', component: Blank, meta: { requiresAuth: false } },
        { path: '/login', name: 'login', component: Blank, meta: { requiresAuth: false } },
        { path: '/profile', name: 'profile', component: Blank, meta: { requiresAuth: true } },
        {
            path: '/admin/users',
            name: 'admin-users',
            component: Blank,
            meta: {
                requiresAuth: true,
                requiresPermission: 'admin:users:read',
            },
        },
    ];
    const router = createRouter({
        history: createMemoryHistory(),
        routes,
    });

    router.beforeEach(authGuard);

    return router;
};

const setLoggedIn = (role: string, permissions: string[] = []): void => {
    user.value = {
        id: 'u-1',
        nickname: 'alice',
        email: '',
        role,
        permissions,
    };
    isAuthenticated.value = true;
};

describe('authGuard', () => {
    beforeEach((): void => {
        clearAuth();
    });

    it('redirects unauthenticated navigation to /profile → /login with redirect query', async (): Promise<void> => {
        const router = makeTestRouter();

        await router.push('/profile');

        expect(router.currentRoute.value.name).toBe('login');
        expect(router.currentRoute.value.query['redirect']).toBe('/profile');
    });

    it('allows authenticated user into /profile', async (): Promise<void> => {
        setLoggedIn('user');
        const router = makeTestRouter();

        await router.push('/profile');

        expect(router.currentRoute.value.name).toBe('profile');
    });

    it('redirects user without permission away from admin route → /home', async (): Promise<void> => {
        setLoggedIn('user', ['profile:read']);
        const router = makeTestRouter();

        await router.push('/admin/users');

        expect(router.currentRoute.value.name).toBe('home');
    });

    it('allows admin into any protected route (has-permission short-circuit)', async (): Promise<void> => {
        setLoggedIn('admin');
        const router = makeTestRouter();

        await router.push('/admin/users');

        expect(router.currentRoute.value.name).toBe('admin-users');
    });

    it('allows user with the required permission listed', async (): Promise<void> => {
        setLoggedIn('user', ['admin:users:read']);
        const router = makeTestRouter();

        await router.push('/admin/users');

        expect(router.currentRoute.value.name).toBe('admin-users');
    });

    it('omits redirect query when protected route is /', async (): Promise<void> => {
        const router = makeTestRouter();

        router.addRoute({
            path: '/',
            name: 'home',
            component: Blank,
            meta: { requiresAuth: true },
        });

        await router.push('/');

        expect(router.currentRoute.value.name).toBe('login');
        expect(router.currentRoute.value.query['redirect']).toBeUndefined();
    });

    it('public routes (requiresAuth: false) pass through without auth', async (): Promise<void> => {
        const router = makeTestRouter();

        await router.push('/');

        expect(router.currentRoute.value.name).toBe('home');
    });
});
