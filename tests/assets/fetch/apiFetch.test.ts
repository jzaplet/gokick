import { describe, expect, it, vi, beforeEach } from 'vitest';
import { apiFetch, setAccessToken } from '@/app-ui/Fetch';

type HealthResponse = {
    status: string;
};

const isHealthResponse = (v: unknown): v is HealthResponse =>
    typeof v === 'object' && v !== null && typeof (v as Record<string, unknown>)['status'] === 'string';

describe('apiFetch', () => {
    beforeEach((): void => {
        setAccessToken(null);
        vi.restoreAllMocks();
    });

    it('returns success response with typed data', async (): Promise<void> => {
        vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(JSON.stringify({ status: 'ok' }), { status: 200 }),
        );

        const result = await apiFetch<HealthResponse>('GET', '/health', { validate: isHealthResponse });

        expect(result.success).toBe(true);
        expect(result.status).toBe(200);

        if (result.success === true) {
            expect(result.data.status).toBe('ok');
        }
    });

    it('returns error response with typed error', async (): Promise<void> => {
        vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(JSON.stringify({ message: 'Unauthorized' }), { status: 401 }),
        );

        const result = await apiFetch<HealthResponse>('GET', '/api/v1/profile', { validate: isHealthResponse });

        expect(result.success).toBe(false);
        expect(result.status).toBe(401);

        if (result.success === false) {
            expect(result.data).toEqual({ message: 'Unauthorized' });
        }
    });

    it('attaches Authorization header when token is set', async (): Promise<void> => {
        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(JSON.stringify({ status: 'ok' }), { status: 200 }),
        );

        setAccessToken('test-token-123');
        await apiFetch<HealthResponse>('GET', '/health', { validate: isHealthResponse });

        const requestInit = fetchSpy.mock.calls[0]?.[1];
        const headers = requestInit?.headers as Record<string, string> | undefined;

        expect(headers?.['Authorization']).toBe('Bearer test-token-123');
    });

    it('omits Authorization header when no token', async (): Promise<void> => {
        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(JSON.stringify({ status: 'ok' }), { status: 200 }),
        );

        await apiFetch<HealthResponse>('GET', '/health', { validate: isHealthResponse });

        const requestInit = fetchSpy.mock.calls[0]?.[1];
        const headers = requestInit?.headers as Record<string, string> | undefined;

        expect(headers?.['Authorization']).toBeUndefined();
    });

    it('uses the provided HTTP method', async (): Promise<void> => {
        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(JSON.stringify({}), { status: 200 }),
        );

        await apiFetch<HealthResponse>('DELETE', '/api/v1/users/123', { validate: isHealthResponse });

        const requestInit = fetchSpy.mock.calls[0]?.[1];

        expect(requestInit?.method).toBe('DELETE');
    });

    it('sends JSON-serialized body', async (): Promise<void> => {
        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(JSON.stringify({}), { status: 200 }),
        );

        type LoginRequest = { nickname: string; password: string };

        const credentials: LoginRequest = { nickname: 'admin', password: 'secret' };

        await apiFetch<HealthResponse, { message: string }, LoginRequest>(
            'POST',
            '/api/v1/auth/login',
            { body: credentials, validate: isHealthResponse },
        );

        const requestInit = fetchSpy.mock.calls[0]?.[1];

        expect(requestInit?.body).toBe('{"nickname":"admin","password":"secret"}');
    });

    it('does NOT retry on 401 (that is authFetch responsibility)', async (): Promise<void> => {
        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(JSON.stringify({ message: 'expired' }), { status: 401 }),
        );

        setAccessToken('stale');
        const result = await apiFetch<HealthResponse>('GET', '/api/v1/profile', { validate: isHealthResponse });

        expect(fetchSpy).toHaveBeenCalledTimes(1);
        expect(result.success).toBe(false);
        expect(result.status).toBe(401);
    });
});
