import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';
import type { FetchOptions } from '@/app-ui/Fetch/types/FetchOptions';
import { apiFetch } from '@/app-ui/Fetch/apiFetch';
import { refresh } from '@/app-ui/Auth/refresh';

// Single-flight refresh — concurrent 401s share one in-flight refresh call
// instead of rotating the token several times in parallel.
let inFlightRefresh: Promise<boolean> | null = null;

const refreshOnce = (): Promise<boolean> => {
    if (inFlightRefresh !== null) {
        return inFlightRefresh;
    }

    inFlightRefresh = refresh().finally(() => {
        inFlightRefresh = null;
    });

    return inFlightRefresh;
};

// apiFetch wrapped with one-shot auto-refresh on 401. Use for every protected
// endpoint; use plain apiFetch directly for endpoints that should never be
// retried (public routes).
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

    const refreshed = await refreshOnce();

    if (refreshed === false) {
        return first;
    }

    return apiFetch<TData, TError>(method, url, options);
};
