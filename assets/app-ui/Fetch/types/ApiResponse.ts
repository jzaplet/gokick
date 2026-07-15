import type { ApiSuccess } from '@/app-ui/Fetch/types/ApiSuccess';
import type { ApiError } from '@/app-ui/Fetch/types/ApiError';

export type ApiResponse<TData, TError> = ApiSuccess<TData> | ApiError<TError>;
