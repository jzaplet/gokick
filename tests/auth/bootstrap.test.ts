import { describe, expect, it, vi, beforeAll, beforeEach } from 'vitest';
import type * as VueModule from 'vue';
import type { bootstrap as Bootstrap } from '@/app';

// guide-forms-fe-36 / guide-auth-perm-40:
// On a hard page refresh, assets/app.ts bootstrap() calls refresh() BEFORE
// mounting the router, so a valid HttpOnly refresh cookie restores the session
// before any route guard runs. We assert the ordering: refresh() is awaited,
// then createApp(App).use(router).mount('#app') happens.
//
// NOTE: assets/app.ts runs `void bootstrap()` at import time, so importing the
// module fires bootstrap once. We clear that import-time invocation, then call
// the exported bootstrap() explicitly under controlled mocks.

const refreshMock = vi.fn().mockResolvedValue(true);
const mountMock = vi.fn();
const useMock = vi.fn(() => ({ mount: mountMock }));
const createAppMock = vi.fn(() => ({ use: useMock }));

// Mock the network/session restore — we only care that it is invoked first.
vi.mock('@/app-ui/Auth/refresh', () => ({
    refresh: refreshMock,
}));

// Partially mock Vue: keep ref/reactive/etc. real (state.ts needs ref via the
// import chain) and override only createApp so mounting never touches the real
// DOM/router. The stub is chainable so .use(router).mount('#app') resolves.
vi.mock('vue', async (importOriginal) => {
    const actual = await importOriginal<typeof VueModule>();

    return {
        ...actual,
        createApp: createAppMock,
    };
});

// Side-effect-only imports in app.ts (css + png asset) — stub so the vite
// asset/css transform is irrelevant under vitest.
vi.mock('@/tailwind.css', () => ({}));
vi.mock('@/img/go-vue-cqrs-ddd.png', () => ({ default: '' }));

describe('app bootstrap', () => {
    let bootstrap: typeof Bootstrap;

    beforeAll(async (): Promise<void> => {
        // Importing @/app fires `void bootstrap()` once. Import here and flush
        // microtasks so that import-time invocation (its refresh + mount) fully
        // settles BEFORE any test runs — then beforeEach can clear it cleanly.
        const mod = await import('@/app');

        bootstrap = mod.bootstrap;
        await Promise.resolve();
        await Promise.resolve();
    });

    beforeEach((): void => {
        // Wipe the calls recorded by the import-time `void bootstrap()` so each
        // test asserts against a clean explicit invocation.
        vi.clearAllMocks();
    });

    it('calls refresh() before mounting the app', async (): Promise<void> => {
        await bootstrap();

        expect(refreshMock).toHaveBeenCalledTimes(1);
        expect(mountMock).toHaveBeenCalledTimes(1);
        expect(mountMock).toHaveBeenCalledWith('#app');

        // vitest has no toHaveBeenCalledBefore — compare invocation order.
        const refreshOrder = refreshMock.mock.invocationCallOrder[0];
        const mountOrder = mountMock.mock.invocationCallOrder[0];

        expect(refreshOrder).toBeDefined();
        expect(mountOrder).toBeDefined();
        expect(Number(refreshOrder)).toBeLessThan(Number(mountOrder));
    });

    it('mounts the created app onto #app', async (): Promise<void> => {
        await bootstrap();

        // createApp(App).use(router).mount('#app') — the full chain ran.
        expect(createAppMock).toHaveBeenCalledTimes(1);
        expect(useMock).toHaveBeenCalledTimes(1);
        expect(mountMock).toHaveBeenCalledWith('#app');
    });
});
