import { getAccessToken } from '@/app-ui/Fetch/accessToken';

// Merges caller-supplied headers with the current Authorization header.
// Returning a fresh object makes this safe to call multiple times per request
// (e.g. before/after a refresh where the access token has changed).
export const buildAuthHeaders = (extra: Record<string, string> = {}): Record<string, string> => {
    const headers: Record<string, string> = { ...extra };

    const token = getAccessToken();

    if (token !== null) {
        headers['Authorization'] = `Bearer ${token}`;
    }

    return headers;
};
