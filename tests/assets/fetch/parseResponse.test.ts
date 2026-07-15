import { describe, expect, it } from 'vitest';
import { parseResponse } from '@/app-ui/Fetch/parseResponse';

type IdBody = { id: string };

const isIdBody = (v: unknown): v is IdBody =>
    typeof v === 'object' && v !== null && typeof (v as Record<string, unknown>)['id'] === 'string';

describe('parseResponse', () => {
    it('a valid 2xx JSON body passing its guard is a success carrying the parsed data', async () => {
        const out = await parseResponse<IdBody, { message: string }>(
            new Response(JSON.stringify({ id: 'u-1' }), { status: 200 }),
            isIdBody,
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

    // The runtime half of the parity loop: a 2xx body that fails its generated
    // guard is a contract violation — a mergeable { general } failure, never data.
    it('a 2xx body failing its guard is a general failure, not data', async () => {
        const out = await parseResponse<IdBody, { message: string }>(
            new Response(JSON.stringify({ id: 42 }), { status: 200 }),
            isIdBody,
        );

        expect(out.success).toBe(false);
        if (out.success === false) {
            expect(out.data).toEqual({ general: 'Invalid response shape' });
        }
    });

    // F-081: a 200 that carries a malformed body is NOT a session — it must be a
    // failure, not swallowed into a fake success with null data.
    it('a malformed 2xx body is a general failure, not a swallowed fake success', async () => {
        const out = await parseResponse<IdBody, { message: string }>(
            new Response('{ not json', { status: 200 }),
            isIdBody,
        );

        expect(out.success).toBe(false);
        if (out.success === false) {
            expect('general' in out.data ? out.data.general : '').toContain('Malformed');
        }
    });

    it('a non-2xx JSON error body is carried through as the error', async () => {
        const out = await parseResponse<unknown, { message: string }>(
            new Response(JSON.stringify({ message: 'nope' }), { status: 400 }),
        );

        expect(out.success).toBe(false);
        if (out.success === false) {
            expect(out.data).toEqual({ message: 'nope' });
        }
    });

    it('a non-2xx EMPTY body has no TError to carry — general failure', async () => {
        const out = await parseResponse<unknown, { message: string }>(
            new Response(null, { status: 502 }),
        );

        expect(out.success).toBe(false);
        if (out.success === false) {
            expect(out.data).toEqual({ general: 'Error 502' });
        }
    });
});
