import type { TypedFetchFn } from '@/app-ui/Fetch/types/TypedFetchFn';
import { apiFetchCore } from '@/app-ui/Fetch/apiFetchCore';

// Plain HTTP JSON fetch with automatic Authorization header, behind the
// shared TypedFetchFn contract (validate required whenever TData ≠ null).
// Deliberately has NO refresh/retry logic — that concern belongs to authFetch
// in the Auth layer, which orchestrates this function together with refresh().
export const apiFetch: TypedFetchFn = apiFetchCore;
