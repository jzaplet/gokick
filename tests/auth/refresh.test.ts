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

    it('does not throw and keeps the hint on a malformed 200 body', async (): Promise<void> => {
        setHint();
        // 200 with an empty/non-JSON body → parseResponse yields { success:true,
        // data:null }; the shape guard rejects it (data === null) so refresh()
        // resolves false without ever touching the access token.
        vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(null, { status: 200 }));

        await expect(refresh()).resolves.toBe(false);
        expect(hasSessionHint()).toBe(true);
        expect(isAuthenticated.value).toBe(false);
    });

    it('rejects a partial 200 body (missing expiration) without a hot loop', async (): Promise<void> => {
        setHint();
        // 200 whose body lacks access_expiration: the field access would be NaN
        // and scheduleRefresh(NaN) would fire setTimeout immediately, spinning a
        // hot retry loop. The shape guard rejects it before any state is set.
        vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(
                JSON.stringify({ access_token: 'x', user: { id: 'u-1' } }),
                { status: 200 },
            ),
        );

        await expect(refresh()).resolves.toBe(false);
        // No session was established and the hint is kept (transient miss).
        expect(isAuthenticated.value).toBe(false);
        expect(getAccessToken()).toBe(null);
        expect(hasSessionHint()).toBe(true);
    });

    // The user principal must be validated too, not just present: an empty or
    // array `user` (typeof [] === 'object') would otherwise flip isAuthenticated
    // true and then crash the router guard at user.permissions.includes(...).
    it.each([
        ['empty user object', {}],
        ['user is an array', []],
        ['user missing permissions', { id: 'u-1', nickname: 'a', email: 'a@x.io', role: 'user' }],
    ])('rejects a 200 whose user principal is malformed (%s)', async (_label, badUser): Promise<void> => {
        setHint();
        vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(
                JSON.stringify({ access_token: 'x', access_expiration: 900, user: badUser }),
                { status: 200 },
            ),
        );

        await expect(refresh()).resolves.toBe(false);
        expect(isAuthenticated.value).toBe(false);
        expect(getAccessToken()).toBe(null);
        expect(hasSessionHint()).toBe(true);
    });
});
