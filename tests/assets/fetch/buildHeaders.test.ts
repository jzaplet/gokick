import { beforeEach, describe, expect, it } from 'vitest';
import { setAccessToken } from '@/app-ui/Fetch';
import { buildAuthHeaders } from '@/app-ui/Fetch/buildHeaders';
import { applyLocale, chooseLocale, initLocale } from '@/app-ui/I18n';

const clearLangCookie = (): void => {
    document.cookie = 'gk_lang=; path=/; max-age=0';
};

describe('buildAuthHeaders', () => {
    beforeEach((): void => {
        setAccessToken(null);
        clearLangCookie();
        document.documentElement.lang = 'en';
        initLocale();
    });

    // X-App-Lang means "the user chose this", not "this is the current
    // locale". Sending it unconditionally makes every request look explicit,
    // which pins the server at the top of its resolution ladder and leaves the
    // persisted users.lang unreachable.
    it('omits X-App-Lang when the browser made no explicit choice', () => {
        applyLocale('cs');

        expect(buildAuthHeaders()['X-App-Lang']).toBeUndefined();
    });

    it('sends the explicit choice, not the active locale', () => {
        chooseLocale('cs');

        expect(buildAuthHeaders()['X-App-Lang']).toBe('cs');
    });

    it('carries the bearer token and the caller-supplied headers', () => {
        setAccessToken('tok');

        const headers = buildAuthHeaders({ 'Content-Type': 'application/json' });

        expect(headers['Authorization']).toBe('Bearer tok');
        expect(headers['Content-Type']).toBe('application/json');
    });
});
