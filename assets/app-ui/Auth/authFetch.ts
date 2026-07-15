import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';
import type { FetchOptions } from '@/app-ui/Fetch/types/FetchOptions';
import type { Guard } from '@/app-ui/Fetch/guards';
import { apiFetchCore } from '@/app-ui/Fetch/apiFetchCore';
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
//
// Same contract as apiFetch: TData ≠ null requires `validate` (the generated
// guard); TData = null (204 endpoints) takes none.
type AuthFetchFn = {
    <TData extends null, TError = { message: string }, TBody = never>(
        method: string,
        url: string,
        options?: FetchOptions<TBody> & { validate?: never },
    ): Promise<ApiResponse<TData, TError>>;
    <TData, TError = { message: string }, TBody = never>(
        method: string,
        url: string,
        options: FetchOptions<TBody> & { validate: Guard<TData> },
    ): Promise<ApiResponse<TData, TError>>;
};

export const authFetch: AuthFetchFn = async <TData, TError, TBody>(
    method: string,
    url: string,
    options: FetchOptions<TBody> & { validate?: Guard<TData> } = {},
): Promise<ApiResponse<TData, TError>> => {
    // The loose implementation signature forwards without a cast; this
    // function's own public overloads already enforce the validate↔TData
    // contract at every call site.
    const first = await apiFetchCore<TData, TError, TBody>(method, url, options);

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

    return apiFetchCore<TData, TError, TBody>(method, url, options);
};
