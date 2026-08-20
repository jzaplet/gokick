// The ONE parser/writer for the app's readable cookies (gk_lang, gk_session).
// Values here are plain tokens the app writes itself, so nothing is
// encoded/decoded. Robust parse: split on ';', trim, split at the FIRST '='
// so a value containing '=' survives.
export const readCookie = (name: string): string | null => {
    for (const part of document.cookie.split(';')) {
        const trimmed = part.trim();
        const eq = trimmed.indexOf('=');

        if (eq !== -1 && trimmed.slice(0, eq) === name) {
            return trimmed.slice(eq + 1);
        }
    }

    return null;
};

export const writeCookie = (name: string, value: string, maxAgeSeconds: number): void => {
    document.cookie = `${name}=${value}; Path=/; Max-Age=${String(maxAgeSeconds)}; SameSite=Lax`;
};

// Deletion matches on name + path (SameSite plays no part), so Max-Age=0 at
// Path=/ removes any cookie the two writers above (or the server, for
// gk_session) set.
export const deleteCookie = (name: string): void => {
    document.cookie = `${name}=; Path=/; Max-Age=0; SameSite=Lax`;
};
