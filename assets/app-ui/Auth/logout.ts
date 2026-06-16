import { apiFetch } from '@/app-ui/Fetch/apiFetch';
import { clearAuth } from '@/app-ui/Auth/state';
import { clearSessionHint } from '@/app-ui/Auth/sessionHint';

// POST /api/v1/auth/logout — server wipes all refresh tokens of the user
// and returns 204; we then clear the local session. Errors on the network
// still clear local state so the user isn't "stuck" logged in.
export const logout = async (): Promise<void> => {
    // try/finally so a network failure (apiFetch rejecting) still clears the
    // local session — otherwise the comment above would be a lie and the user
    // would stay "logged in" in the SPA after a failed logout POST.
    try {
        await apiFetch<null>('POST', '/api/v1/auth/logout');
    } finally {
        clearAuth();
        // Logout is explicit intent to end the session, so drop the gk_session
        // hint regardless of whether the POST reached the server — otherwise a
        // network-failed logout would leave the hint and the next page load
        // would silently restore the session.
        clearSessionHint();
    }
};
