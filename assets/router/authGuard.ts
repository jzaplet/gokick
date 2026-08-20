import type { NavigationGuard, RouteLocationNormalized, RouteLocationRaw } from 'vue-router';
import { hasPermission, isAuthenticated } from '@/app-ui/Auth';
import {
    applyLocale,
    CANONICAL_LANG,
    getLocale,
    isLang,
    toLangParam,
    useI18n,
} from '@/app-ui/I18n';
import { useToast } from '@/app-ui/Toast/useToast';

// redirectWithLang re-resolves the target route with a different `lang`
// param ('' clears the optional prefix), preserving everything else.
//
// Deliberately NO `replace` flag: a guard redirect without its own flag
// inherits the original navigation's type (vue-router merges `{ replace }`
// from the interrupted navigation under the redirect location), and the very
// first navigation always replaces internally. So initial-load
// canonicalization still leaves history clean, while an in-app push (every
// named navigation resolves the bare path and lands here) stays a push —
// an unconditional `replace: true` made Back skip pages.
const redirectWithLang = (to: RouteLocationNormalized, langParam: string): RouteLocationRaw => ({
    name: to.name ?? undefined,
    params: { ...to.params, lang: langParam },
    query: to.query,
    hash: to.hash,
});

// langRedirect handles the canonical-URL rules: the canonical
// language lives at the bare path, other languages carry their prefix.
// Returns a redirect location or null when the URL is already canonical.
//
//   /en/x  → strip the prefix (English is canonical bare)
//   /cs/x  → sync the locale from the URL and continue
//   /x     → when the effective locale is not canonical (explicit flag
//            choice, adopted profile preference, or the server-negotiated
//            shell language from Accept-Language), add the prefix — this is
//            what redirects a first-time Czech-browser visit of "/" to "/cs".
const langRedirect = (to: RouteLocationNormalized): RouteLocationRaw | null => {
    const raw = to.params['lang'];
    const langParam = typeof raw === 'string' ? raw : '';

    if (langParam === CANONICAL_LANG) {
        applyLocale(CANONICAL_LANG);

        return redirectWithLang(to, '');
    }
    if (isLang(langParam)) {
        applyLocale(langParam);

        return null;
    }
    if (getLocale() !== CANONICAL_LANG) {
        return redirectWithLang(to, toLangParam(getLocale()));
    }

    return null;
};

// Navigation guard that keeps language-prefixed URLs canonical and enforces
// `meta.requiresAuth` and `meta.requiresPermission`. Both meta fields come
// from AppRoute (see meta.ts) and are statically required by TypeScript, so
// no runtime fallback is needed — a missing meta.requiresAuth is a
// compile-time error.
export const authGuard: NavigationGuard = (to) => {
    const { error, info } = useToast();
    const { t } = useI18n();

    const redirect = langRedirect(to);

    if (redirect !== null) {
        return redirect;
    }

    if (to.meta.requiresAuth === true && isAuthenticated.value === false) {
        info(t('auth.sign_in_to_continue'));

        return {
            name: 'login',
            query: to.fullPath === '/' ? {} : { redirect: to.fullPath },
        };
    }

    if (to.meta.requiresPermission !== undefined
        && hasPermission(to.meta.requiresPermission) === false) {
        error(t('auth.no_permission'));

        return { name: 'home' };
    }

    return true;
};
