// The server sets a readable `gk_session=1` cookie next to the HttpOnly refresh
// cookie, with the same lifetime. JS can't see the HttpOnly cookie, so this hint
// lets the bootstrap skip the session-restore POST /api/v1/auth/refresh — and
// its guaranteed 401 — when no session plausibly exists. The name mirrors the
// backend's sessionHintCookieName.
const cookieName = 'gk_session';

export const hasSessionHint = (): boolean =>
    document.cookie.split('; ').some((c) => c === `${cookieName}=1`);

// Drop the hint locally after any auth teardown (logout, refresh failure, 401
// give-up), so a server-revoked-but-not-yet-expired session doesn't keep
// re-triggering the bootstrap refresh. Safe — clearAuth only runs when the
// session is already gone, so there is never a real session to preserve.
export const clearSessionHint = (): void => {
    document.cookie = `${cookieName}=; Path=/; Max-Age=0; SameSite=Strict`;
};
