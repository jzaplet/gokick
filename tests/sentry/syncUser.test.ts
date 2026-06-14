import { describe, expect, it, vi, beforeAll, beforeEach } from 'vitest';
import { nextTick } from 'vue';
import { syncSentryUser } from '@/app-ui/Sentry/syncUser';
import { user } from '@/app-ui/Auth/state';

// vi.hoisted so the spy exists before the (also hoisted) vi.mock factory runs —
// syncUser.ts imports @sentry/vue at module load. vitest lifts both above the
// imports, so the static imports here still receive the mocked setUser.
const setUserMock = vi.hoisted(() => vi.fn());

vi.mock('@sentry/vue', () => ({ setUser: setUserMock }));

const alice = {
    id: 'u-7',
    nickname: 'alice',
    email: 'alice@example.com',
    role: 'admin',
    permissions: [],
};

describe('syncSentryUser', () => {
    beforeAll((): void => {
        // One watcher for the whole suite — it lives for the app's lifetime in
        // production, so we mirror that instead of re-creating it per test.
        syncSentryUser();
    });

    beforeEach(async (): Promise<void> => {
        // Reset the shared session ref and flush the resulting watcher run, so
        // each test asserts against a clean Sentry call log.
        user.value = null;
        await nextTick();
        setUserMock.mockClear();
    });

    it('maps the session user onto Sentry when login/refresh populates it', async (): Promise<void> => {
        user.value = { ...alice };
        await nextTick();

        expect(setUserMock).toHaveBeenCalledTimes(1);
        expect(setUserMock).toHaveBeenCalledWith({
            id: 'u-7',
            username: 'alice',
            email: 'alice@example.com',
            role: 'admin',
        });
    });

    it('clears the Sentry user when the session is wiped on logout', async (): Promise<void> => {
        user.value = { ...alice };
        await nextTick();
        setUserMock.mockClear();

        user.value = null;
        await nextTick();

        expect(setUserMock).toHaveBeenCalledTimes(1);
        expect(setUserMock).toHaveBeenCalledWith(null);
    });
});
