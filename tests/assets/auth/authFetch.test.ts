import { describe, expect, it, vi, beforeEach } from 'vitest';
import { authFetch } from '@/app-ui/Auth/authFetch';
import { clearAuth } from '@/app-ui/Auth/state';
import { setAccessToken } from '@/app-ui/Fetch/accessToken';

type HealthResponse = {
    status: string;
};

// Integration-style: only fetch() is spied, everything else (refresh,
// state, apiFetch) runs for real — which is exactly what the real flow
// does at runtime.
describe('authFetch', () => {
    beforeEach((): void => {
        clearAuth();
        vi.restoreAllMocks();
    });

    it('returns immediately on non-401 responses without calling refresh', async (): Promise<void> => {
        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(JSON.stringify({ status: 'ok' }), { status: 200 }),
        );

        const result = await authFetch<HealthResponse>('GET', '/api/v1/profile');

        expect(fetchSpy).toHaveBeenCalledTimes(1);
        expect(result.success).toBe(true);
    });

    it('retries once after a successful refresh with the new access token', async (): Promise<void> => {
        const refreshResponse = {
            access_token: 'new-access-token',
            access_expiration: 900,
            user: { id: 'u-1', nickname: 'alice', email: 'alice@example.com', role: 'user', permissions: [] },
        };
        const responses: Response[] = [
            new Response(JSON.stringify({ message: 'expired' }), { status: 401 }),
            new Response(JSON.stringify(refreshResponse), { status: 200 }),
            new Response(JSON.stringify({ status: 'ok' }), { status: 200 }),
        ];
        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(
            (): Promise<Response> => Promise.resolve(responses.shift() ?? new Response(null, { status: 500 })),
        );

        setAccessToken('stale-token');

        const result = await authFetch<HealthResponse>('GET', '/api/v1/profile');

        // 3 network calls: initial 401, /auth/refresh, retried /profile.
        expect(fetchSpy).toHaveBeenCalledTimes(3);
        expect(fetchSpy.mock.calls[1]?.[0]).toBe('/api/v1/auth/refresh');

        const retryHeaders = fetchSpy.mock.calls[2]?.[1]?.headers as Record<string, string> | undefined;

        expect(retryHeaders?.['Authorization']).toBe('Bearer new-access-token');
        expect(result.success).toBe(true);
    });

    it('returns the original 401 when refresh fails', async (): Promise<void> => {
        const responses: Response[] = [
            new Response(JSON.stringify({ message: 'expired' }), { status: 401 }),
            new Response(JSON.stringify({ message: 'no refresh cookie' }), { status: 401 }),
        ];
        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(
            (): Promise<Response> => Promise.resolve(responses.shift() ?? new Response(null, { status: 500 })),
        );

        setAccessToken('stale');

        const result = await authFetch<HealthResponse>('GET', '/api/v1/profile');

        // Initial call + refresh attempt, no retry (refresh failed).
        expect(fetchSpy).toHaveBeenCalledTimes(2);
        expect(result.success).toBe(false);
        expect(result.status).toBe(401);
    });

    it('does not retry auth endpoints (login/refresh/logout)', async (): Promise<void> => {
        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(JSON.stringify({ message: 'invalid credentials' }), { status: 401 }),
        );

        const result = await authFetch<HealthResponse>('POST', '/api/v1/auth/login', {
            body: { nickname: 'x', password: 'y' },
        });

        expect(fetchSpy).toHaveBeenCalledTimes(1);
        expect(result.success).toBe(false);
    });

    it('coalesces concurrent 401s into a single refresh call', async (): Promise<void> => {
        const refreshResponse = {
            access_token: 'fresh',
            access_expiration: 900,
            user: { id: 'u-1', nickname: 'alice', email: 'alice@example.com', role: 'user', permissions: [] },
        };
        let refreshCalls = 0;

        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(
            async (input, init): Promise<Response> => {
                const url = typeof input === 'string' ? input : (input as URL).toString();
                const headers = (init?.headers ?? {}) as Record<string, string>;
                const token = headers['Authorization'];

                if (url === '/api/v1/auth/refresh') {
                    refreshCalls += 1;
                    // Simulate a short network delay so concurrent callers queue up.
                    await Promise.resolve();

                    return new Response(JSON.stringify(refreshResponse), { status: 200 });
                }

                if (token === 'Bearer stale') {
                    return new Response(JSON.stringify({ message: 'expired' }), { status: 401 });
                }

                return new Response(JSON.stringify({ status: 'ok' }), { status: 200 });
            },
        );

        setAccessToken('stale');

        const [a, b, c] = await Promise.all([
            authFetch<HealthResponse>('GET', '/api/v1/profile'),
            authFetch<HealthResponse>('GET', '/api/v1/admin/users'),
            authFetch<HealthResponse>('GET', '/api/v1/settings'),
        ]);

        // 3 initial 401s + 1 shared refresh + 3 retries = 7 fetch calls.
        expect(refreshCalls).toBe(1);
        expect(fetchSpy).toHaveBeenCalledTimes(7);
        expect(a.success).toBe(true);
        expect(b.success).toBe(true);
        expect(c.success).toBe(true);
    });
});
