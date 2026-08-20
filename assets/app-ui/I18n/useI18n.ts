import { computed, ref } from 'vue';
import type { ComputedRef } from 'vue';
import { readCookie, writeCookie } from '@/app-ui/Cookies/cookies';
import type { ApiMessage } from '@/app-ui/Fetch/types/ApiMessage';
import { canonicalCatalog, catalogs } from '@/app-ui/I18n/catalogs.gen';
import { CANONICAL_LANG, isLang } from '@/app-ui/I18n/lang';
import type {
    Lang,
    PluralMessage,
    TranslationKey,
    TranslationMessage,
    TranslationParams,
} from '@/app-ui/I18n/lang';
import { reportUnexpected } from '@/app-ui/Sentry/reportUnexpected';

// The explicit language choice lives in a READABLE cookie (not localStorage)
// on purpose: the Go server uses it to serve the right <html lang> shell
// variant and to resolve headerless requests, and a future SSR landing will
// use it for exact server-side redirects. One year, Lax — it carries no
// secret. The name mirrors shared.LangCookieName on the backend.
const LANG_COOKIE = 'gk_lang';
const LANG_COOKIE_MAX_AGE = 60 * 60 * 24 * 365;

// Module-level singleton — every component shares one locale, mirroring the
// useToast store pattern.
const locale = ref<Lang>(CANONICAL_LANG);

// Intl.PluralRules construction is expensive; one instance per language
// serves every t() call.
const pluralRules = new Map<Lang, Intl.PluralRules>();

const rulesFor = (lang: Lang): Intl.PluralRules => {
    const cached = pluralRules.get(lang);

    if (cached !== undefined) {
        return cached;
    }
    const created = new Intl.PluralRules(lang);

    pluralRules.set(lang, created);

    return created;
};

// Same memoisation for number formatting: a translated message that carries a
// number must print it the way the ACTIVE language writes numbers, not the way
// String() does. Czech groups with a narrow space and uses a decimal comma, so
// String(1048576) / String(1.5) would render "1048576" / "1.5" inside otherwise
// perfectly Czech sentences.
const numberFormats = new Map<Lang, Intl.NumberFormat>();

const numberFormatFor = (lang: Lang): Intl.NumberFormat => {
    const cached = numberFormats.get(lang);

    if (cached !== undefined) {
        return cached;
    }
    const created = new Intl.NumberFormat(lang);

    numberFormats.set(lang, created);

    return created;
};

const readLangCookie = (): Lang | null => {
    const value = readCookie(LANG_COOKIE);

    return isLang(value) ? value : null;
};

const writeLangCookie = (lang: Lang): void => {
    writeCookie(LANG_COOKIE, lang, LANG_COOKIE_MAX_AGE);
};

// applyLocale switches the session locale without recording a choice — used
// by URL-prefix sync and server-preference adoption.
export const applyLocale = (lang: Lang): void => {
    locale.value = lang;
    document.documentElement.lang = lang;
};

// hasExplicitChoice reports whether this browser picked a language via the
// switcher (the gk_lang cookie) — a stored choice outranks the profile
// preference and Accept-Language.
const hasExplicitChoice = (): boolean => readLangCookie() !== null;

// The browser-negotiated boot baseline. adoptServerLang falls back to it when
// a profile carries NO language, so on a shared cookie-less browser user B
// does not inherit the locale user A's profile adopted.
//
// The served <html lang> is only a valid baseline when the URL carried no
// /<lang> prefix: the shell reflects the server's FULL resolution, and a
// prefix outranks everything in it. A prefix the previous session left in the
// address bar would otherwise be recorded here as "what this browser
// negotiated" and handed to the next user — exactly the leak this baseline
// exists to prevent.
let negotiatedLang: Lang = CANONICAL_LANG;

// initLocale runs once at bootstrap, before the router mounts: cookie choice
// first, then the server-negotiated <html lang>, then the canonical default.
export const initLocale = (): void => {
    const docLang = document.documentElement.lang;
    const [, firstSegment = ''] = window.location.pathname.split('/');
    const fromUrlPrefix = isLang(firstSegment);

    if (isLang(docLang) === true && fromUrlPrefix === false) {
        negotiatedLang = docLang;
    } else {
        negotiatedLang = CANONICAL_LANG;
    }

    const stored = readLangCookie();

    if (stored !== null) {
        applyLocale(stored);

        return;
    }
    applyLocale(isLang(docLang) === true ? docLang : negotiatedLang);
};

// chooseLocale records an explicit user choice (flag/switcher click).
export const chooseLocale = (lang: Lang): void => {
    writeLangCookie(lang);
    applyLocale(lang);
};

// adoptServerLang applies the persisted profile preference from login/refresh
// — only when this browser has not chosen explicitly. A profile WITHOUT a
// preference ("" / unsupported) re-applies the negotiated boot baseline
// instead of keeping whatever locale the previous profile adopted, so a
// cookie-less shared browser never leaks user A's language to user B.
export const adoptServerLang = (lang: string): void => {
    if (hasExplicitChoice() === true) {
        return;
    }
    applyLocale(isLang(lang) ? lang : negotiatedLang);
};

export const getLocale = (): Lang => locale.value;

// getExplicitLocale returns the language this browser CHOSE (the gk_lang
// cookie), or null when the active locale was negotiated, adopted from a
// profile, or taken from a URL prefix. The fetch layer sends X-App-Lang only
// for a real choice — a header on every request would make the server treat
// each one as explicit and never reach the user's persisted users.lang
// (ladder: header → cookie → users.lang → Accept-Language → en).
export const getExplicitLocale = (): Lang | null => readLangCookie();

const pluralForm = (message: PluralMessage, count: number, lang: Lang): string => {
    const rule = rulesFor(lang).select(count);

    return message[rule] ?? message.other;
};

// Hoisted so String.replace does not re-allocate the regex per t() call
// (replace with a /g/ regex always matches from index 0, so sharing is safe).
const PLACEHOLDER_RE = /\{(\w+)\}/g;

// Params reach here typed from t() and raw (Record<string, unknown>, the
// generated ApiMessage shape) from tm() — the scalar check below is the ONE
// gate for both, so nothing walks the params object per render. Only
// string/number can interpolate; anything else leaves its {placeholder}
// visible rather than being String()-coerced into the UI — the same
// debuggability contract as t()'s raw-key fallback. Numbers go through the
// active language's Intl.NumberFormat, so grouping and the decimal separator
// match the sentence around them.
const interpolate = (
    template: string,
    lang: Lang,
    params?: Record<string, unknown>,
): string => {
    if (params === undefined) {
        return template;
    }

    return template.replace(PLACEHOLDER_RE, (match: string, name: string): string => {
        const value = params[name];

        if (typeof value === 'string') {
            return value;
        }
        if (typeof value === 'number') {
            return numberFormatFor(lang).format(value);
        }

        return match;
    });
};

// render is the shared renderer behind t() (typed params) and tm() (wire
// params) — it takes the widest param type so neither caller has to copy or
// re-key the object.
//
// Misuse never throws and never half-renders (backend parity): a missing
// message (typed-unreachable, crash-proofed anyway) or a plural key called
// without a numeric `count` returns the RAW KEY and reports the bug — a
// visible `users.bulk.deleted` beats a silently wrong plural with a literal
// `{count}` left in the output.
const render = (key: TranslationKey, params?: Record<string, unknown>): string => {
    // The catalog type is complete by construction, so this lookup "cannot"
    // miss — the widened Partial view is what lets the runtime crash-proofing
    // exist without a cast when reality disagrees with the types.
    const catalog: Partial<Record<string, TranslationMessage>> = catalogs[locale.value];
    const message = catalog[key];

    if (message === undefined) {
        reportUnexpected(`i18n: missing message for key "${key}"`);

        return key;
    }
    if (typeof message === 'string') {
        return interpolate(message, locale.value, params);
    }
    const count = params?.['count'];

    if (typeof count !== 'number') {
        reportUnexpected(`i18n: plural key "${key}" called without a numeric count param`);

        return key;
    }

    return interpolate(pluralForm(message, count, locale.value), locale.value, params);
};

// t renders a catalog message in the active locale — the typed public entry.
// Reads the locale ref, so components using it in templates re-render on a
// locale switch. Importable directly by non-component modules (fetch layer,
// helpers); components conventionally go through useI18n().
export const t = (key: TranslationKey, params?: TranslationParams): string => render(key, params);

// isTranslationKey narrows a wire-supplied key to the closed TranslationKey
// union. The canonical catalog is complete by construction, so membership in
// it IS the definition of "known key" — no cast needed.
const isTranslationKey = (key: string): key is TranslationKey => key in canonicalCatalog;

// tm renders a backend wire message ({ key, params }) in the active locale —
// the display half of keys-over-the-wire. null/undefined pass through as
// null so `tm(errors.field)` slots straight into nullable :error props.
// An unknown key mirrors t()'s misuse contract: report the bug and render
// the raw key — a visible `user.nickname_required` beats a blank. Wire
// params pass through untouched (interpolate is the scalar gate), so a
// keystroke-frequency re-render allocates nothing here. Reactive like t()
// (it reads the locale through render), so rendered errors flip on a
// language switch.
export const tm = (message: ApiMessage | null | undefined): string | null => {
    if (message === null || message === undefined) {
        return null;
    }
    const { key, params } = message;

    if (isTranslationKey(key) === false) {
        reportUnexpected(`i18n: unknown wire message key "${key}"`);

        return key;
    }

    return render(key, params);
};

type UseI18n = {
    t: typeof t;
    locale: ComputedRef<Lang>;
    chooseLocale: typeof chooseLocale;
};

const localeComputed = computed((): Lang => locale.value);

// One shared instance — nothing in the API is per-caller, so useI18n() is a
// free singleton read even when called per grid row.
const i18nApi: UseI18n = { t, locale: localeComputed, chooseLocale };

export const useI18n = (): UseI18n => i18nApi;
