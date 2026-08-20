import { CANONICAL_LANG, SUPPORTED_LANGS } from '@/app-ui/I18n/langs.gen';
import type { Lang } from '@/app-ui/I18n/langs.gen';
import type { canonicalCatalog } from '@/app-ui/I18n/catalogs.gen';

// Language plumbing. The language SET is generated: SUPPORTED_LANGS,
// the Lang union and CANONICAL_LANG come from langs.gen.ts, which `gk i18n`
// writes from the locale/messages.<lang>.json filenames — the same source the
// backend's shared.SupportedLangs is written from, so the two cannot drift and
// there is nothing to declare here. Adding a language is a catalog file plus
// `make i18n-gen`; it flows to the router prefix, the switcher and the guard
// automatically.
//
// Re-exported rather than imported straight from the generated module so the
// public surface stays this hand-written file: moving or renaming an artifact
// is then a normal TS refactor, invisible to every consumer.
export { CANONICAL_LANG, SUPPORTED_LANGS };
export type { Lang };

// toLangParam maps a language to its router `lang` param value — '' clears
// the optional prefix for the canonical language.
export const toLangParam = (lang: Lang): string => (lang === CANONICAL_LANG ? '' : lang);

// A message is either plain text or CLDR plural forms; `other` is mandatory,
// the remaining forms are per-language (Czech uses one/few, English one).
export type PluralMessage = Partial<Record<Intl.LDMLPluralRule, string>> & { other: string };

export type TranslationMessage = string | PluralMessage;

// The canonical key union — a call site typo, or a key missing from another
// catalog, is a compile error. Derived from the GENERATED canonical catalog
// but declared here: the public type surface is hand-written, so a moved or
// renamed catalog file is a normal TS refactor. The import is type-only and
// erases, so the mutual reference with catalogs.gen.ts is not a runtime cycle.
export type TranslationKey = keyof typeof canonicalCatalog;

// Every non-canonical catalog is annotated with this, so vue-tsc rejects a
// missing or extra key on the frontend side too.
export type TranslationCatalog = Record<TranslationKey, TranslationMessage>;

// Interpolation values for {name} placeholders; the `count` param additionally
// selects the plural form.
export type TranslationParams = Record<string, string | number>;

export const isLang = (value: string | null | undefined): value is Lang => {
    return SUPPORTED_LANGS.some((lang): boolean => lang === value);
};

// stripLangSegment removes a leading supported-language segment from an
// absolute pathname ('/cs/x' → '/x', '/cs' → '/'); anything else passes
// through untouched.
const stripLangSegment = (pathname: string): string => {
    const [, first = '', ...rest] = pathname.split('/');

    if (isLang(first) === false) {
        return pathname === '' ? '/' : pathname;
    }
    const remainder = rest.join('/');

    return remainder === '' ? '/' : `/${remainder}`;
};

// localizePath rebases an absolute in-app path onto a language: strips any
// existing supported-lang prefix (first path segment only) and prepends the
// target language's segment ('' for canonical). Query and hash survive
// untouched — needed wherever a stored path (a ?redirect param, a hardcoded
// reload target) is navigated to AFTER the locale may have changed.
export const localizePath = (path: string, lang: Lang): string => {
    const splitAt = path.search(/[?#]/);
    const pathname = splitAt === -1 ? path : path.slice(0, splitAt);
    const suffix = splitAt === -1 ? '' : path.slice(splitAt);
    const bare = stripLangSegment(pathname);
    const param = toLangParam(lang);

    if (param === '') {
        return `${bare}${suffix}`;
    }

    return bare === '/' ? `/${param}${suffix}` : `/${param}${bare}${suffix}`;
};
