import { describe, expect, it, vi, beforeEach } from 'vitest';
import { refresh } from '@/app-ui/Auth/refresh';
import { clearAuth, isAuthenticated } from '@/app-ui/Auth/state';
import { hasSessionHint } from '@/app-ui/Auth/sessionHint';
import { getAccessToken } from '@/app-ui/Fetch/accessToken';

// code-review finding #1: the gk_session hint must be dropped ONLY at the
// definitive end of a session (a 401 from refresh), never on a transient 5xx or
// a network error — otherwise a momentary backend hiccup deletes the hint, the
// next bootstrap skips refresh, and a still-valid session is durably logged out.
const loginBody = {
    access_token: 'fresh-access-token',
    access_expiration: 900,
    user: { id: 'u-1', nickname: 'alice', email: 'alice@example.com', role: 'user', permissions: [] },
};

const setHint = (): void => {
    document.cookie = 'gk_session=1; Path=/';
};

describe('refresh — session hint lifecycle', () => {
    beforeEach((): void => {
        clearAuth();
        document.cookie = 'gk_session=; Path=/; Max-Age=0';
        vi.restoreAllMocks();
    });

    it('restores the session and KEEPS the hint on success', async (): Promise<void> => {
        setHint();
        vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(JSON.stringify(loginBody), { status: 200 }),
        );

        const ok = await refresh();

        expect(ok).toBe(true);
        expect(getAccessToken()).toBe('fresh-access-token');
        expect(isAuthenticated.value).toBe(true);
        expect(hasSessionHint()).toBe(true);
    });

    it('clears the hint on a definitive 401 (token invalid/revoked)', async (): Promise<void> => {
        setHint();
        vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(JSON.stringify({ message: 'invalid' }), { status: 401 }),
        );

        const ok = await refresh();

        expect(ok).toBe(false);
        expect(hasSessionHint()).toBe(false);
        expect(isAuthenticated.value).toBe(false);
    });

    it('KEEPS the hint on a transient 5xx — no durable logout', async (): Promise<void> => {
        setHint();
        vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(JSON.stringify({ message: 'boom' }), { status: 503 }),
        );

        const ok = await refresh();

        expect(ok).toBe(false);
        // The crux of finding #1: a momentary 5xx must not delete the hint, so
        // the next page load can still self-heal instead of staying logged out.
        expect(hasSessionHint()).toBe(true);
    });

    it('KEEPS the hint and never throws on a network error', async (): Promise<void> => {
        setHint();
        vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('offline'));

        // refresh() is total — a transport failure resolves to false, not a throw.
        await expect(refresh()).resolves.toBe(false);
        expect(hasSessionHint()).toBe(true);
    });
});
