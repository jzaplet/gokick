import { describe, expect, it, beforeEach } from 'vitest';
import { hasSessionHint, clearSessionHint } from '@/app-ui/Auth/sessionHint';

describe('sessionHint', () => {
    beforeEach((): void => {
        document.cookie = 'gk_session=; Path=/; Max-Age=0';
    });

    it('reports a session when the gk_session=1 cookie is present', (): void => {
        document.cookie = 'gk_session=1; Path=/';

        expect(hasSessionHint()).toBe(true);
    });

    it('reports no session when the cookie is absent (a guest)', (): void => {
        expect(hasSessionHint()).toBe(false);
    });

    it('clearSessionHint removes the cookie so the next load skips the restore', (): void => {
        document.cookie = 'gk_session=1; Path=/';
        expect(hasSessionHint()).toBe(true);

        clearSessionHint();

        expect(hasSessionHint()).toBe(false);
    });
});
