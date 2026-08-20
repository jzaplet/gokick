import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { RouteLocationNormalized } from 'vue-router';
import { applyLocale } from '@/app-ui/I18n';
import { authGuard } from '@/router/authGuard';

// Minimal RouteLocationNormalized stand-in — the guard only reads name,
// params, query, hash, fullPath and meta.
const makeTo = (lang: string | undefined): RouteLocationNormalized => {
    const to: Partial<RouteLocationNormalized> = {
        name: 'home',
        params: lang === undefined ? {} : { lang },
        query: {},
        hash: '',
        fullPath: lang === undefined ? '/' : `/${lang}`,
        path: lang === undefined ? '/' : `/${lang}`,
        matched: [],
        meta: { requiresAuth: false },
        redirectedFrom: undefined,
    };

    return to as RouteLocationNormalized;
};

const runGuard = (to: RouteLocationNormalized): unknown => authGuard(to, makeTo(undefined), vi.fn());

const clearLangCookie = (): void => {
    document.cookie = 'gk_lang=; path=/; max-age=0';
};

beforeEach((): void => {
    clearLangCookie();
    document.documentElement.lang = 'en';
    applyLocale('en');
});

describe('authGuard language canonicalization', () => {
    it('strips the /en prefix (English is canonical bare)', () => {
        const result = runGuard(makeTo('en'));

        expect(result).toMatchObject({ params: { lang: '' } });
        // No own `replace` flag — the redirect must inherit the original
        // navigation's type (push stays push) so Back keeps working.
        expect(result).not.toHaveProperty('replace');
    });

    it('syncs the locale from a /cs prefix and continues', () => {
        const result = runGuard(makeTo('cs'));

        expect(result).toBe(true);
        expect(document.documentElement.lang).toBe('cs');
    });

    it('adds the prefix when the effective locale is not English', () => {
        applyLocale('cs');

        const result = runGuard(makeTo(undefined));

        expect(result).toMatchObject({ params: { lang: 'cs' } });
        expect(result).not.toHaveProperty('replace');
    });

    it('passes a bare URL through for the English locale', () => {
        const result = runGuard(makeTo(undefined));

        expect(result).toBe(true);
    });
});
