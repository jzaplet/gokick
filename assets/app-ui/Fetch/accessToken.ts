// Access token is stored in memory only (deliberately — XSS-resistant).
// A single module-level variable is shared across the whole SPA.

let accessToken: string | null = null;

export const setAccessToken = (token: string | null): void => {
    accessToken = token;
};

export const getAccessToken = (): string | null => {
    return accessToken;
};
