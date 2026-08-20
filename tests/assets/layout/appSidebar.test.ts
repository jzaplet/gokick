import { describe, expect, it, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import { createMemoryHistory, createRouter } from 'vue-router';
import type { Router } from 'vue-router';
import AppSidebar from '@/app/Layout/AppSidebar.vue';
import { clearAuth, user } from '@/app-ui/Auth';
import type { Role } from '@/app/Auth/enums/roles';

// The sidebar owns the role-aware nav (moved from the old top header): a
// superadmin sees ONLY the platform plane, an admin sees admin tools, a plain
// user sees just their dashboard. These tests pin that routing logic — the
// permission strings come from the server-supplied user.permissions.

const routeNames = [
    'home',
    'platform-dashboard',
    'platform-tenants',
    'platform-users',
    'admin-dashboard',
    'admin-users',
    'user-dashboard',
];

const makeRouter = (): Router => createRouter({
    history: createMemoryHistory(),
    routes: routeNames.map((name) => ({
        name,
        path: name === 'home' ? '/' : `/${name}`,
        component: { template: '<div />' },
    })),
});

const seedUser = (role: Role, permissions: string[]): void => {
    user.value = { id: 'u-1', nickname: 'alice', email: '', role, permissions, lang: '' };
};

type SidebarWrapper = ReturnType<typeof mount<typeof AppSidebar>>;

const mountSidebar = (): SidebarWrapper => mount(AppSidebar, {
    global: { plugins: [makeRouter()] },
});

const linkLabels = (wrapper: SidebarWrapper): string[] =>
    wrapper.findAll('nav a').map((a) => a.text());

describe('AppSidebar', () => {
    beforeEach((): void => {
        clearAuth();
        localStorage.clear();
    });

    it('shows only the platform links to a superadmin', () => {
        seedUser('superadmin', ['platform:overview']);

        const wrapper = mountSidebar();

        expect(linkLabels(wrapper)).toEqual(['Dashboard', 'Tenants', 'Users']);
        expect(wrapper.text()).toContain('Platform');
    });

    it('shows admin dashboard and users to an admin', () => {
        seedUser('admin', ['admin:dashboard:read', 'admin:users:read']);

        const wrapper = mountSidebar();

        expect(linkLabels(wrapper)).toEqual(['Dashboard', 'Users']);
        expect(wrapper.text()).toContain('Application');
    });

    it('shows just the user dashboard to a plain user', () => {
        seedUser('user', []);

        const wrapper = mountSidebar();

        expect(linkLabels(wrapper)).toEqual(['Dashboard']);
    });
});
