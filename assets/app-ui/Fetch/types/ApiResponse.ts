import type { ApiSuccess } from '@/app-ui/Fetch/types/ApiSuccess';
import type { ApiError } from '@/app-ui/Fetch/types/ApiError';
import type { ApiTransport } from '@/app-ui/Fetch/types/ApiTransport';

export type ApiResponse<TData, TError> = ApiSuccess<TData> | ApiError<TError> | ApiTransport;

// Narrow a failed result: transport-level failure (network / malformed body /
// contract violation — carries only `message`) vs an API error carrying the
// endpoint's TError shape. Reading `result.data` on a failure does not compile
// until the transport case is ruled out.
export const isTransport = <TData, TError>(
    r: ApiResponse<TData, TError>,
): r is ApiTransport => r.success === false && 'transport' in r;
