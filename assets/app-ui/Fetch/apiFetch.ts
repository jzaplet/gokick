import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';
import type { FetchOptions } from '@/app-ui/Fetch/types/FetchOptions';
import type { Guard } from '@/app-ui/Fetch/guards';
import { apiFetchCore } from '@/app-ui/Fetch/apiFetchCore';

// The public call shape: endpoints that RETURN DATA (TData ≠ null) must pass
// `validate` — the tsgen-generated guard for TData — so the body is checked
// against the generated contract at runtime; 204/no-content endpoints
// (TData = null) take no guard (there is nothing to validate) and may omit
// options entirely. The pairing guard↔generic is compiler-checked
// (Guard<TData>), so declaring AdminUser[] and passing isPlatformUser fails.
type ApiFetchFn = {
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

// Plain HTTP JSON fetch with automatic Authorization header.
// Deliberately has NO refresh/retry logic — that concern belongs to authFetch
// in the Auth layer, which orchestrates this function together with refresh().
export const apiFetch: ApiFetchFn = apiFetchCore;
