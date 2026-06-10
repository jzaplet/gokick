import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import type { MockInstance } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import { createMemoryHistory, createRouter } from 'vue-router';
import type { Router } from 'vue-router';
import AdminUserEditView from '@/app/Admin/Views/AdminUserEditView.vue';
import UserForm from '@/app/Admin/Components/UserForm.vue';
import type { UserFormData } from '@/app/Admin/types/UserFormData';
import { clearAuth, user } from '@/app-ui/Auth';

// Concrete type of vi.spyOn(router, 'push') — annotating submitForm's return with
// this (rather than the overloaded ReturnType<typeof vi.spyOn>, which lint widens
// to `any`) keeps `const pushSpy = await submitForm(...)` assignment-safe.
type PushSpy = MockInstance<Router['push']>;

// roadmap-96: changing the ROLE on one's OWN account must force a full-page
// reload (window.location.assign) so bootstrap re-runs refresh() and the SPA
// picks up the freshly-minted access token. Every other edit path must use a
// normal in-SPA router.push and NOT reload.
//
// Integration-style: only fetch() is mocked. authFetch / apiFetch / the real
// router all run, which is what happens at runtime. The mounted onMounted GET
// loads the editable user (so initial.role is populated), then the form's
// submit event drives handleSubmit.

const TARGET_ID = 'u-self';
const OTHER_ID = 'u-other';

const Blank = { template: '<div />' };

// One user list shared by every GET. The edited row starts as role "user".
const userList = [
    { id: TARGET_ID, nickname: 'alice', email: 'alice@x.dev', role: 'user', active: true },
    { id: OTHER_ID, nickname: 'bob', email: 'bob@x.dev', role: 'user', active: true },
];

// Routes the network: GET list -> userList, PUT update -> 200 success.
const mockFetch = (): void => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(
        (input, init): Promise<Response> => {
            const url = typeof input === 'string' ? input : (input as URL).toString();
            const method = (init?.method ?? 'GET').toUpperCase();

            if (method === 'PUT' && url.startsWith('/api/v1/admin/users/')) {
                return Promise.resolve(new Response(JSON.stringify(null), { status: 200 }));
            }

            if (method === 'GET' && url === '/api/v1/admin/users') {
                return Promise.resolve(new Response(JSON.stringify(userList), { status: 200 }));
            }

            return Promise.resolve(new Response(null, { status: 500 }));
        },
    );
};

const makeRouter = (editId: string): Router => {
    const router = createRouter({
        history: createMemoryHistory(),
        routes: [
            { path: '/admin/users', name: 'admin-users', component: Blank, meta: { requiresAuth: true } },
            { path: '/admin/users/:id/edit', name: 'admin-users-edit', component: Blank, meta: { requiresAuth: true } },
        ],
    });

    void router.push(`/admin/users/${editId}/edit`);

    return router;
};

// jsdom throws "Not implemented: navigation" on a real location.assign, so we
// install a fresh spy-backed location for the duration of each test.
let assignSpy: ReturnType<typeof vi.fn>;
let originalLocation: Location;

const installLocationStub = (): void => {
    originalLocation = window.location;
    assignSpy = vi.fn();
    // Build the stub explicitly rather than spreading the Location instance
    // (no-misused-spread: spreading a class instance drops its prototype). The
    // view only calls window.location.assign; the readable string fields are
    // copied across so any incidental reads still see the original values.
    Object.defineProperty(window, 'location', {
        configurable: true,
        writable: true,
        value: {
            assign: assignSpy,
            href: originalLocation.href,
            origin: originalLocation.origin,
            pathname: originalLocation.pathname,
            search: originalLocation.search,
            hash: originalLocation.hash,
        },
    });
};

const restoreLocationStub = (): void => {
    Object.defineProperty(window, 'location', {
        configurable: true,
        writable: true,
        value: originalLocation,
    });
};

// Mounts the view, lets the onMounted GET resolve, then emits the form's
// submit event with `data`. Returns the spy on router.push.
const submitForm = async (
    router: Router,
    data: UserFormData,
): Promise<PushSpy> => {
    await router.isReady();

    const wrapper = mount(AdminUserEditView, {
        global: {
            plugins: [router],
            stubs: ['Spinner'],
        },
    });

    // Let the onMounted GET settle so `initial` is populated and UserForm renders.
    await flushPromises();

    const pushSpy = vi.spyOn(router, 'push');
    const form = wrapper.findComponent(UserForm);

    expect(form.exists()).toBe(true);

    // UserForm declares `submit: [data: UserFormData]`, but @vue/test-utils types
    // an SFC wrapper's $emit as `any` under type-aware lint, so cast it once to the
    // real signature rather than calling the `any` member directly.
    // eslint-disable-next-line @typescript-eslint/no-unsafe-member-access -- SFC wrapper $emit is typed `any`
    const emitSubmit = form.vm.$emit as (event: 'submit', data: UserFormData) => void;

    emitSubmit('submit', data);
    await flushPromises();

    return pushSpy;
};

describe('AdminUserEditView role-change reload (roadmap-96)', () => {
    beforeEach((): void => {
        clearAuth();
        mockFetch();
        installLocationStub();
    });

    afterEach((): void => {
        restoreLocationStub();
        vi.restoreAllMocks();
    });

    it('editing OWN account and CHANGING role forces a full-page reload to /admin/users', async (): Promise<void> => {
        // Current user is the one being edited.
        user.value = { id: TARGET_ID, nickname: 'alice', email: '', role: 'user', permissions: [] };

        const router = makeRouter(TARGET_ID);
        const pushSpy = await submitForm(router, {
            nickname: 'alice',
            password: '',
            email: 'alice@x.dev',
            role: 'admin', // changed from "user"
        });

        expect(assignSpy).toHaveBeenCalledTimes(1);
        expect(assignSpy).toHaveBeenCalledWith('/admin/users');
        // Reload path returns early — no SPA navigation.
        expect(pushSpy).not.toHaveBeenCalled();
    });

    it('editing OWN account WITHOUT changing role uses router.push, no reload', async (): Promise<void> => {
        user.value = { id: TARGET_ID, nickname: 'alice', email: '', role: 'user', permissions: [] };

        const router = makeRouter(TARGET_ID);
        const pushSpy = await submitForm(router, {
            nickname: 'alice-renamed',
            password: '',
            email: 'alice@x.dev',
            role: 'user', // unchanged
        });

        expect(assignSpy).not.toHaveBeenCalled();
        expect(pushSpy).toHaveBeenCalledWith({ name: 'admin-users' });
    });

    it('editing a DIFFERENT user and changing their role uses router.push, no reload', async (): Promise<void> => {
        // Logged in as TARGET_ID, but editing OTHER_ID — not self.
        user.value = { id: TARGET_ID, nickname: 'alice', email: '', role: 'admin', permissions: [] };

        const router = makeRouter(OTHER_ID);
        const pushSpy = await submitForm(router, {
            nickname: 'bob',
            password: '',
            email: 'bob@x.dev',
            role: 'admin', // changed for bob, but bob is not the current user
        });

        expect(assignSpy).not.toHaveBeenCalled();
        expect(pushSpy).toHaveBeenCalledWith({ name: 'admin-users' });
    });

    it('does NOT reload when the PUT fails (errors surface, no navigation)', async (): Promise<void> => {
        // Self + role change, but the server rejects the update.
        user.value = { id: TARGET_ID, nickname: 'alice', email: '', role: 'user', permissions: [] };

        vi.spyOn(globalThis, 'fetch').mockImplementation(
            (input, init): Promise<Response> => {
                const url = typeof input === 'string' ? input : (input as URL).toString();
                const method = (init?.method ?? 'GET').toUpperCase();

                if (method === 'PUT' && url.startsWith('/api/v1/admin/users/')) {
                    return Promise.resolve(
                        new Response(JSON.stringify({ role: 'invalid role' }), { status: 400 }),
                    );
                }

                if (method === 'GET' && url === '/api/v1/admin/users') {
                    return Promise.resolve(new Response(JSON.stringify(userList), { status: 200 }));
                }

                return Promise.resolve(new Response(null, { status: 500 }));
            },
        );

        const router = makeRouter(TARGET_ID);
        const pushSpy = await submitForm(router, {
            nickname: 'alice',
            password: '',
            email: 'alice@x.dev',
            role: 'admin',
        });

        expect(assignSpy).not.toHaveBeenCalled();
        expect(pushSpy).not.toHaveBeenCalled();
    });
});
