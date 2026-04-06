import { describe, expect, it, vi, beforeEach } from 'vitest';
import { apiFetch, setAccessToken } from '@/app-ui/Fetch/useFetch';

describe('useFetch', () => {
    beforeEach((): void => {
        setAccessToken(null);
        vi.restoreAllMocks();
    });

    it('returns success response with data', async (): Promise<void> => {
        vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(JSON.stringify({ status: 'ok' }), {
                status: 200,
                headers: { 'Content-Type': 'application/json' },
            }),
        );

        const result = await apiFetch<{ status: string }>('/health');

        expect(result.success).toBe(true);
        if (result.success === true) {
            expect(result.data.status).toBe('ok');
        }
        expect(result.status).toBe(200);
    });

    it('returns error response on non-ok status', async (): Promise<void> => {
        vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(JSON.stringify({ message: 'Unauthorized' }), {
                status: 401,
                headers: { 'Content-Type': 'application/json' },
            }),
        );

        const result = await apiFetch<unknown>('/auth/profile');

        expect(result.success).toBe(false);
        if (result.success === false) {
            expect(result.data.message).toBe('Unauthorized');
        }
        expect(result.status).toBe(401);
    });

    it('sends Authorization header when token is set', async (): Promise<void> => {
        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(JSON.stringify({}), { status: 200 }),
        );

        setAccessToken('test-token-123');
        await apiFetch<unknown>('/profile');

        const [, init] = fetchSpy.mock.calls[0] as [string, RequestInit];
        const headers = init.headers as Record<string, string>;

        expect(headers['Authorization']).toBe('Bearer test-token-123');
    });

    it('does not send Authorization header when no token', async (): Promise<void> => {
        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(JSON.stringify({}), { status: 200 }),
        );

        await apiFetch<unknown>('/health');

        const [, init] = fetchSpy.mock.calls[0] as [string, RequestInit];
        const headers = init.headers as Record<string, string>;

        expect(headers['Authorization']).toBeUndefined();
    });

    it('prefixes url with /api/v1', async (): Promise<void> => {
        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(JSON.stringify({}), { status: 200 }),
        );

        await apiFetch<unknown>('/auth/login');

        const [url] = fetchSpy.mock.calls[0] as [string];

        expect(url).toBe('/api/v1/auth/login');
    });

    it('sends JSON body for POST requests', async (): Promise<void> => {
        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(JSON.stringify({}), { status: 200 }),
        );

        await apiFetch<unknown>('/auth/login', {
            method: 'POST',
            body: { nickname: 'admin', password: 'secret' },
        });

        const [, init] = fetchSpy.mock.calls[0] as [string, RequestInit];

        expect(init.method).toBe('POST');
        expect(init.body).toBe('{"nickname":"admin","password":"secret"}');
    });
});
