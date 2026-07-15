import type { ApiGeneralError } from '@/app-ui/Fetch/types/ApiGeneralError';
import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';
import type { FetchOptions } from '@/app-ui/Fetch/types/FetchOptions';
import type { Guard } from '@/app-ui/Fetch/guards';

// The ONE public call contract shared by both fetch facades (apiFetch,
// authFetch) — a single type on purpose, so the two can never drift apart
// silently (they used to be two verbatim copies).
//
// Endpoints that RETURN DATA (TData ≠ null) must pass `validate` — the
// tsgen-generated guard for TData — so the body is checked against the
// generated contract at runtime. 204/no-content endpoints (TData = null)
// take no guard (there is nothing to validate) and may omit options
// entirely. The pairing guard↔generic is compiler-checked (Guard<TData>),
// so declaring AdminUser[] and passing isPlatformUser fails.
//
// TError defaults to ApiGeneralError: since failures merged on the
// TError | ApiGeneralError union, { general } is the one shape every failure
// path can actually produce — the old { message: string } default described
// a body no failure path emits, so narrowing on it could never fire.
export type TypedFetchFn = {
    <TData extends null, TError = ApiGeneralError, TBody = never>(
        method: string,
        url: string,
        options?: FetchOptions<TBody> & { validate?: never },
    ): Promise<ApiResponse<TData, TError>>;
    <TData, TError = ApiGeneralError, TBody = never>(
        method: string,
        url: string,
        options: FetchOptions<TBody> & { validate: Guard<TData> },
    ): Promise<ApiResponse<TData, TError>>;
};
