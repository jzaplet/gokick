import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { TranslationKey } from '@/app-ui/I18n';
import {
    adoptServerLang,
    applyLocale,
    chooseLocale,
    getLocale,
    initLocale,
    tm,
    useI18n,
} from '@/app-ui/I18n';

// t() reports catalog misuse through the one sanctioned FE reporting path;
// the mock both silences it and lets the misuse tests assert it fired.
const reportMock = vi.hoisted(() => vi.fn());

vi.mock('@/app-ui/Sentry/reportUnexpected', () => ({ reportUnexpected: reportMock }));

const clearLangCookie = (): void => {
    document.cookie = 'gk_lang=; path=/; max-age=0';
};

beforeEach((): void => {
    reportMock.mockReset();
    clearLangCookie();
    document.documentElement.lang = 'en';
    initLocale();
    applyLocale('en');
});

describe('t', () => {
    it('translates in the active locale and re-reads it on switch', () => {
        const { t } = useI18n();

        expect(t('common.cancel')).toBe('Cancel');
        applyLocale('cs');
        expect(t('common.cancel')).toBe('Zrušit');
    });

    it('interpolates named params', () => {
        const { t } = useI18n();

        expect(t('users.created', { nickname: 'bob' })).toBe('User bob created.');
    });

    it('selects Czech plural forms by count (1 / 2–4 / 5+)', () => {
        const { t } = useI18n();

        applyLocale('cs');
        expect(t('users.bulk.deleted', { count: 1 })).toBe('1 uživatel byl smazán.');
        expect(t('users.bulk.deleted', { count: 3 })).toBe('3 uživatelé byli smazáni.');
        expect(t('users.bulk.deleted', { count: 8 })).toBe('8 uživatelů bylo smazáno.');
    });

    it('selects English plural forms by count', () => {
        const { t } = useI18n();

        expect(t('users.bulk.deleted', { count: 1 })).toBe('1 user deleted.');
        expect(t('users.bulk.deleted', { count: 5 })).toBe('5 users deleted.');
    });

    it('carries extra params alongside the plural count', () => {
        const { t } = useI18n();

        expect(t('tenants.bulk.deleted_skipped', { count: 2, skipped: 1 }))
            .toBe('2 tenants deleted, 1 skipped (they still have users).');
    });

    it('selects the Czech many form for fractional counts', () => {
        const { t } = useI18n();

        applyLocale('cs');
        // Czech writes the decimal separator as a comma — a number interpolated
        // into a Czech sentence must be formatted for Czech, not String()-ed.
        expect(t('users.bulk.deleted', { count: 1.5 })).toBe('1,5 uživatele bylo smazáno.');
    });

    it('formats interpolated numbers for the active locale', () => {
        const { t } = useI18n();

        applyLocale('en');
        expect(t('users.bulk.deleted', { count: 1234 })).toBe('1,234 users deleted.');

        applyLocale('cs');
        // Asserted against Intl itself rather than a literal: Czech groups
        // thousands with a space whose exact code point (U+00A0 vs U+202F)
        // moves between ICU versions — what must hold is that the number goes
        // through the locale's formatter at all.
        const grouped = new Intl.NumberFormat('cs').format(1234);

        expect(grouped).not.toBe('1234');
        expect(t('users.bulk.deleted', { count: 1234 })).toBe(`${grouped} uživatelů bylo smazáno.`);
    });

    it('returns the raw key and reports when a plural key gets no count', () => {
        const { t } = useI18n();

        expect(t('users.bulk.deleted')).toBe('users.bulk.deleted');
        expect(reportMock).toHaveBeenCalledTimes(1);
    });

    it('returns the raw key and reports when count is not a number', () => {
        const { t } = useI18n();

        expect(t('users.bulk.deleted', { count: '5' })).toBe('users.bulk.deleted');
        expect(reportMock).toHaveBeenCalledTimes(1);
    });

    it('returns the raw key and reports when the message is missing at runtime', () => {
        const { t } = useI18n();
        // TranslationKey is a closed union, so a missing message is
        // typed-unreachable — the cast (allowed in tests/) simulates the
        // runtime hole the guard crash-proofs.
        const missingKey = 'nope.missing' as unknown as TranslationKey;

        expect(t(missingKey)).toBe('nope.missing');
        expect(reportMock).toHaveBeenCalledTimes(1);
    });
});

describe('tm', () => {
    it('renders a known wire key in the active locale and follows a switch', () => {
        expect(tm({ key: 'common.cancel' })).toBe('Cancel');
        applyLocale('cs');
        expect(tm({ key: 'common.cancel' })).toBe('Zrušit');
    });

    it('interpolates wire params', () => {
        expect(tm({ key: 'users.created', params: { nickname: 'bob' } })).toBe('User bob created.');
    });

    it('selects the plural form via the count param', () => {
        expect(tm({ key: 'users.bulk.deleted', params: { count: 1 } })).toBe('1 user deleted.');
        expect(tm({ key: 'users.bulk.deleted', params: { count: 5 } })).toBe('5 users deleted.');
    });

    it('leaves the placeholder visible for a non-scalar param', () => {
        // A non-string/number wire value is never String()-coerced into the
        // UI — same debuggability contract as t()'s raw-key fallback.
        expect(tm({ key: 'users.created', params: { nickname: { nested: true } } }))
            .toBe('User {nickname} created.');
    });

    it('returns the raw key and reports on an unknown wire key', () => {
        expect(tm({ key: 'nope.unknown_wire_key' })).toBe('nope.unknown_wire_key');
        expect(reportMock).toHaveBeenCalledTimes(1);
    });

    it('passes null and undefined through as null', () => {
        expect(tm(null)).toBeNull();
        expect(tm(undefined)).toBeNull();
        expect(reportMock).not.toHaveBeenCalled();
    });
});

describe('locale resolution', () => {
    it('chooseLocale records the choice in the gk_lang cookie', () => {
        chooseLocale('cs');

        expect(document.cookie).toContain('gk_lang=cs');
        expect(getLocale()).toBe('cs');
        expect(document.documentElement.lang).toBe('cs');
    });

    it('initLocale prefers the cookie choice over the document language', () => {
        chooseLocale('cs');
        document.documentElement.lang = 'en';
        applyLocale('en');

        initLocale();

        expect(getLocale()).toBe('cs');
    });

    it('initLocale falls back to the server-negotiated html lang', () => {
        document.documentElement.lang = 'cs';

        initLocale();

        expect(getLocale()).toBe('cs');
    });

    it('adoptServerLang applies the profile preference when no choice exists', () => {
        adoptServerLang('cs');

        expect(getLocale()).toBe('cs');
        expect(document.cookie).not.toContain('gk_lang=cs');
    });

    it('adoptServerLang never overrides an explicit device choice', () => {
        chooseLocale('en');

        adoptServerLang('cs');

        expect(getLocale()).toBe('en');
    });

    it('adoptServerLang with no profile preference re-applies the negotiated baseline', () => {
        // Boot negotiated an English shell...
        document.documentElement.lang = 'en';
        initLocale();

        // ...user A's profile adopted Czech during their session...
        adoptServerLang('cs');
        expect(getLocale()).toBe('cs');

        // ...user B (profile lang NULL) signs in on the same cookie-less
        // browser: the locale returns to the baseline instead of leaking cs.
        adoptServerLang('');
        expect(getLocale()).toBe('en');
    });

    it('a /<lang> URL prefix never becomes the negotiated baseline', () => {
        // The shell reflects the server's FULL resolution, and a /cs prefix
        // outranks everything in it — so <html lang="cs"> here says what the
        // URL asked for, not what this browser negotiated. Recording it would
        // hand the prefix (which the previous session may have left in the
        // address bar) to the next user as their default.
        window.history.pushState({}, '', '/cs/admin');
        document.documentElement.lang = 'cs';
        initLocale();

        // The page still renders in the language it was served in...
        expect(getLocale()).toBe('cs');

        // ...but a profile carrying NO preference falls back to the canonical
        // default rather than inheriting the prefix.
        adoptServerLang('');
        expect(getLocale()).toBe('en');

        window.history.pushState({}, '', '/');
    });

    it('adoptServerLang treats an unsupported preference like none', () => {
        document.documentElement.lang = 'cs';
        initLocale();

        adoptServerLang('de');
        expect(getLocale()).toBe('cs');
    });

    it('adoptServerLang never resets past an explicit device choice', () => {
        document.documentElement.lang = 'en';
        initLocale();
        chooseLocale('cs');

        adoptServerLang('');
        expect(getLocale()).toBe('cs');
    });
});
