import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';
import type { FetchOptions } from '@/app-ui/Fetch/types/FetchOptions';
import { buildAuthHeaders } from '@/app-ui/Fetch/buildHeaders';
import { parseResponse } from '@/app-ui/Fetch/parseResponse';

// Plain HTTP JSON fetch with automatic Authorization header.
// Deliberately has NO refresh/retry logic — that concern belongs to authFetch
// in the Auth layer, which orchestrates this function together with refresh().
//
// TBody defaults to `never` (see FetchOptions): sending a body requires
// declaring the request type explicitly — apiFetch<Res, Err, Req>.
export const apiFetch = async <TData, TError = { message: string }, TBody = never>(
    method: string,
    url: string,
    options: FetchOptions<TBody> = {},
): Promise<ApiResponse<TData, TError>> => {
    const init: RequestInit = {
        method,
        headers: buildAuthHeaders({
            'Content-Type': 'application/json',
            ...options.headers,
        }),
        credentials: 'same-origin',
    };

    if (options.body !== undefined) {
        init.body = JSON.stringify(options.body);
    }

    let response: Response;

    try {
        response = await fetch(url, init);
    } catch {
        // Transport/network failure (offline, DNS, CORS, aborted) — surface it as
        // a structured ApiError so every `result.success === false` consumer fires
        // (toast, clear isLoading) instead of an unhandled promise rejection.
        // status:0 marks "never reached the server"; refresh() treats it as a
        // transient failure that keeps the session. `as TError` matches the same
        // fetch-boundary cast parseResponse uses for the error body.
        return { success: false, status: 0, data: { message: 'network error' } as TError };
    }

    return parseResponse<TData, TError>(response);
};
