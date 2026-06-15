// The server sets a readable `gk_session=1` cookie next to the HttpOnly refresh
// cookie, with the same lifetime. JS can't see the HttpOnly cookie, so this hint
// lets the bootstrap skip the session-restore POST /api/v1/auth/refresh — and
// its guaranteed 401 — when no session plausibly exists. The name mirrors the
// backend's sessionHintCookieName.
const cookieName = 'gk_session';

export const hasSessionHint = (): boolean =>
    document.cookie.split('; ').some((c) => c === `${cookieName}=1`);

// Drop the hint locally at the definitive end of a session — an explicit logout,
// or a 401 from refresh (the token is invalid/revoked). NOT on a transient
// 5xx/offline refresh: that keeps the hint so the next load can self-heal rather
// than stay stuck logged out. See logout.ts / refresh.ts.
export const clearSessionHint = (): void => {
    document.cookie = `${cookieName}=; Path=/; Max-Age=0; SameSite=Strict`;
};
