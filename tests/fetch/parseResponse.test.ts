import { describe, expect, it } from 'vitest';
import { parseResponse } from '@/app-ui/Fetch/parseResponse';

describe('parseResponse', () => {
    it('a valid 2xx JSON body is a success carrying the parsed data', async () => {
        const out = await parseResponse<{ id: string }, { message: string }>(
            new Response(JSON.stringify({ id: 'u-1' }), { status: 200 }),
        );

        expect(out.success).toBe(true);
        if (out.success === true) {
            expect(out.data.id).toBe('u-1');
        }
    });

    it('an empty 2xx body (204) is a bodyless success, not a failure', async () => {
        const out = await parseResponse<unknown, { message: string }>(
            new Response(null, { status: 204 }),
        );

        expect(out.success).toBe(true);
    });

    // F-081: a 200 that carries a malformed body is NOT a session — it must be a
    // failure, not swallowed into a fake success with null data.
    it('a malformed 2xx body is a failure, not a swallowed fake success', async () => {
        const out = await parseResponse<{ id: string }, { message: string }>(
            new Response('{ not json', { status: 200 }),
        );

        expect(out.success).toBe(false);
        if (out.success === false) {
            expect(out.data.message).toContain('Malformed');
        }
    });

    it('a non-2xx JSON error body is carried through as the error', async () => {
        const out = await parseResponse<unknown, { message: string }>(
            new Response(JSON.stringify({ message: 'nope' }), { status: 400 }),
        );

        expect(out.success).toBe(false);
        if (out.success === false) {
            expect(out.data.message).toBe('nope');
        }
    });
});
