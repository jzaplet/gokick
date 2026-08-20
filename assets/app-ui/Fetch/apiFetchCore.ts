import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';
import type { FetchOptions } from '@/app-ui/Fetch/types/FetchOptions';
import type { Guard } from '@/app-ui/Fetch/guards';
import { buildAuthHeaders } from '@/app-ui/Fetch/buildHeaders';
import { generalFailure, parseResponse } from '@/app-ui/Fetch/parseResponse';

// The IMPLEMENTATION of apiFetch, with the loose signature (validate optional,
// no TData↔validate coupling). It exists as its own module so authFetch —
// whose own public overloads already enforce the validate↔TData contract at
// every call site — can forward to it without a cast. App code never imports
// this; it uses the strict apiFetch facade (see apiFetch.ts).
export const apiFetchCore = async <TData, TError, TBody>(
    method: string,
    url: string,
    options: FetchOptions<TBody> & { validate?: Guard<TData> } = {},
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
        // Transport/network failure (offline, DNS, CORS, aborted) — surface it
        // as a synthesized { general } failure so every failure consumer fires
        // (field merge, toast, clear isLoading) instead of an unhandled promise
        // rejection. status:0 marks "never reached the server"; refresh()
        // treats it as a transient failure that keeps the session.
        return generalFailure<TError>(0, 'fetch.network_error');
    }

    return parseResponse<TData, TError>(response, options.validate);
};
