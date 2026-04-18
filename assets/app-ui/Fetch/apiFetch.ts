import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';
import type { FetchOptions } from '@/app-ui/Fetch/types/FetchOptions';
import { buildAuthHeaders } from '@/app-ui/Fetch/buildHeaders';
import { parseResponse } from '@/app-ui/Fetch/parseResponse';

// Plain HTTP JSON fetch with automatic Authorization header.
// Deliberately has NO refresh/retry logic — that concern belongs to authFetch
// in the Auth layer, which orchestrates this function together with refresh().
export const apiFetch = async <TData, TError = { message: string }>(
    method: string,
    url: string,
    options: FetchOptions = {},
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

    const response = await fetch(url, init);

    return parseResponse<TData, TError>(response);
};
