import { describe, expect, it, vi, beforeEach } from 'vitest';
import { logout } from '@/app-ui/Auth/logout';
import { clearAuth, isAuthenticated, user } from '@/app-ui/Auth/state';
import { hasSessionHint } from '@/app-ui/Auth/sessionHint';
import { getAccessToken, setAccessToken } from '@/app-ui/Fetch/accessToken';

// guide-auth-perm-43: Logout deletes the token from the DB, the cookie, and
// memory. The DB delete + cookie clear are the SERVER's job (covered in Go);
// on the FE we assert the "memory" half: logout() POSTs /api/v1/auth/logout
// and then clearAuth() wipes accessToken, user, and isAuthenticated — plus the
// gk_session hint, so a network-failed logout can't leave the hint behind and
// silently restore the session on the next load.
const seedSession = (): void => {
    setAccessToken('live-access-token');
    user.value = { id: 'u-1', nickname: 'alice', email: '', role: 'user', permissions: [] };
    isAuthenticated.value = true;
    document.cookie = 'gk_session=1; Path=/';
};

const expectSessionCleared = (): void => {
    expect(getAccessToken()).toBeNull();
    expect(user.value).toBeNull();
    expect(isAuthenticated.value).toBe(false);
    expect(hasSessionHint()).toBe(false);
};

describe('logout', () => {
    beforeEach((): void => {
        clearAuth();
        document.cookie = 'gk_session=; Path=/; Max-Age=0';
        vi.restoreAllMocks();
    });

    it('POSTs /api/v1/auth/logout then clears the in-memory session', async (): Promise<void> => {
        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(null, { status: 204 }),
        );

        seedSession();
        await logout();

        expect(fetchSpy).toHaveBeenCalledTimes(1);
        expect(fetchSpy.mock.calls[0]?.[0]).toBe('/api/v1/auth/logout');
        expect(fetchSpy.mock.calls[0]?.[1]?.method).toBe('POST');
        expectSessionCleared();
    });

    it('still clears the session when the server returns an error status', async (): Promise<void> => {
        // parseResponse resolves (does not throw) on non-2xx, so the awaited
        // apiFetch returns a failure result and clearAuth() still runs — the
        // user is never left "stuck" logged in on a server-side hiccup.
        vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(JSON.stringify({ message: 'boom' }), { status: 500 }),
        );

        seedSession();
        await logout();

        expectSessionCleared();
    });

    it('still clears the session when the network request rejects', async (): Promise<void> => {
        // logout() is `try { await apiFetch(...) } finally { clearAuth() }`, so a
        // rejecting fetch (offline / DNS failure) propagates out of logout() — but
        // the finally block still wipes local state. Assert BOTH: logout rejects
        // AND the in-memory session is gone, so the user isn't left "stuck".
        vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('network'));

        seedSession();
        await expect(logout()).rejects.toThrow('network');

        expectSessionCleared();
    });
});
