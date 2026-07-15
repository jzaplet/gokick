import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { refresh } from '@/app-ui/Auth/refresh';
import { clearAuth, isAuthenticated } from '@/app-ui/Auth/state';
import { hasSessionHint } from '@/app-ui/Auth/sessionHint';
import { getAccessToken, setAccessToken } from '@/app-ui/Fetch/accessToken';

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

    afterEach((): void => {
        // Some tests below switch to fake timers; restore real ones so a pending
        // backoff timer can't bleed into the next test.
        vi.useRealTimers();
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

    // The core of the deferred "silent refresh" fix: a transient blip during the
    // background auto-refresh must NOT flip a LIVE session to "signed out". The
    // session is kept and a jittered backoff retry self-heals it in place; only a
    // definitive 401 (or an exhausted budget) tears it down.
    it('keeps a live session and retries on a transient 5xx, then self-heals', async (): Promise<void> => {
        vi.useFakeTimers();
        vi.spyOn(Math, 'random').mockReturnValue(0.5); // jitter factor 1.0 → delay = base
        setHint();
        // Simulate the signed-in session the scheduled auto-refresh runs inside.
        isAuthenticated.value = true;
        setAccessToken('old-but-valid');

        const fetchSpy = vi.spyOn(globalThis, 'fetch')
            .mockResolvedValueOnce(new Response(JSON.stringify({ message: 'boom' }), { status: 503 }))
            .mockResolvedValueOnce(new Response(JSON.stringify(loginBody), { status: 200 }));

        const first = await refresh();

        // The crux: the momentary 5xx did NOT log the user out.
        expect(first).toBe(false);
        expect(isAuthenticated.value).toBe(true);
        expect(hasSessionHint()).toBe(true);
        expect(fetchSpy).toHaveBeenCalledTimes(1);

        // The backoff retry (base 2 s) fires and succeeds → session refreshed in place.
        await vi.advanceTimersByTimeAsync(2_000);

        expect(fetchSpy).toHaveBeenCalledTimes(2);
        expect(isAuthenticated.value).toBe(true);
        expect(getAccessToken()).toBe('fresh-access-token');
    });

    // Single-flight is a SECURITY property: concurrent refresh() calls (the timer
    // racing authFetch's 401 retry within one tab) must collapse to ONE rotation,
    // or the backend's compare-and-swap treats the parallel rotations as theft and
    // force-logs-out the session.
    it('coalesces concurrent refresh() calls into a single rotation', async (): Promise<void> => {
        setHint();
        let rotations = 0;

        vi.spyOn(globalThis, 'fetch').mockImplementation(async (): Promise<Response> => {
            rotations += 1;
            await Promise.resolve(); // let the other callers queue on the in-flight promise

            return new Response(JSON.stringify(loginBody), { status: 200 });
        });

        const [a, b, c] = await Promise.all([refresh(), refresh(), refresh()]);

        expect(rotations).toBe(1);
        expect(a).toBe(true);
        expect(b).toBe(true);
        expect(c).toBe(true);
    });

    // After the retry budget is spent the session is finally torn down — but the
    // hint is kept, so the next page load can still restore (no durable logout).
    it('gives up after the retry budget but keeps the hint', async (): Promise<void> => {
        vi.useFakeTimers();
        vi.spyOn(Math, 'random').mockReturnValue(0.5);
        setHint();
        isAuthenticated.value = true;
        setAccessToken('old-but-valid');

        const fetchSpy = vi.spyOn(globalThis, 'fetch')
            .mockResolvedValue(new Response(JSON.stringify({ message: 'down' }), { status: 503 }));

        await refresh(); // attempt 1 → schedules the first retry
        expect(isAuthenticated.value).toBe(true);

        // Drive all 5 retries (2 s, 4 s, 8 s, 16 s, 32 s at jitter factor 1.0).
        for (const ms of [2_000, 4_000, 8_000, 16_000, 32_000]) {
            await vi.advanceTimersByTimeAsync(ms);
        }

        // 1 initial + 5 retries = 6 attempts, then the budget is spent.
        expect(fetchSpy).toHaveBeenCalledTimes(6);
        expect(isAuthenticated.value).toBe(false);
        expect(hasSessionHint()).toBe(true);
        expect(getAccessToken()).toBe(null);
    });
});
