import { apiFetch } from '@/app-ui/Fetch/apiFetch';
import { clearAuth } from '@/app-ui/Auth/state';

// POST /api/v1/auth/logout — server wipes all refresh tokens of the user
// and returns 204; we then clear the local session. Errors on the network
// still clear local state so the user isn't "stuck" logged in.
export const logout = async (): Promise<void> => {
    await apiFetch<unknown>('POST', '/api/v1/auth/logout');
    clearAuth();
};
