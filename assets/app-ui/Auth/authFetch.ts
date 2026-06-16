import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';
import type { FetchOptions } from '@/app-ui/Fetch/types/FetchOptions';
import { apiFetch } from '@/app-ui/Fetch/apiFetch';
import { refresh } from '@/app-ui/Auth/refresh';

// apiFetch wrapped with one-shot auto-refresh on 401. Use for every protected
// endpoint; use plain apiFetch directly for endpoints that should never be
// retried (public routes).
//
// Single-flight lives inside refresh() itself, so concurrent 401s here — and the
// background auto-refresh timer — all share ONE rotation of the cookie. Racing
// rotations within a tab would trip the backend's concurrent-rotation theft
// detection and log the session out.
//
// /api/v1/auth/* is deliberately skipped:
//   - /login 401 means wrong credentials (refresh can't help)
//   - /refresh retrying would infinite-loop
//   - /logout is a one-shot cleanup
export const authFetch = async <TData, TError = { message: string }>(
    method: string,
    url: string,
    options: FetchOptions = {},
): Promise<ApiResponse<TData, TError>> => {
    const first = await apiFetch<TData, TError>(method, url, options);

    if (first.status !== 401) {
        return first;
    }

    if (url.startsWith('/api/v1/auth/')) {
        return first;
    }

    const refreshed = await refresh();

    if (refreshed === false) {
        return first;
    }

    return apiFetch<TData, TError>(method, url, options);
};
