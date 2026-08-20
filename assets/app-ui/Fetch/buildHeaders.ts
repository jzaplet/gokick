import { getAccessToken } from '@/app-ui/Fetch/accessToken';
import { getExplicitLocale } from '@/app-ui/I18n';

// Merges caller-supplied headers with the current Authorization header and,
// when this browser picked a language explicitly, that choice (X-App-Lang).
// The API ships translation KEYS and the frontend renders them, so
// the header does not localize response bodies — it tells the server which
// shell variant to serve and which language to stamp on background runs.
//
// Sent ONLY for an explicit choice, on purpose: an unconditional header makes
// every request look explicit, which pins the server's resolution at the top
// rung and leaves the user's persisted users.lang unreachable. Without the
// header the server falls through to that preference, then Accept-Language.
//
// Returning a fresh object makes this safe to call multiple times per request
// (e.g. before/after a refresh where the access token has changed).
export const buildAuthHeaders = (extra: Record<string, string> = {}): Record<string, string> => {
    const headers: Record<string, string> = { ...extra };

    const chosen = getExplicitLocale();

    if (chosen !== null) {
        headers['X-App-Lang'] = chosen;
    }

    const token = getAccessToken();

    if (token !== null) {
        headers['Authorization'] = `Bearer ${token}`;
    }

    return headers;
};
