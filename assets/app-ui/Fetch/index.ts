// Public entry point for the Fetch layer.
// Views MAY import from here for endpoints that don't need authentication
// (/health, public pages, …). For protected API calls use authFetch from Auth.

export { apiFetch } from '@/app-ui/Fetch/apiFetch';
export { apiUpload } from '@/app-ui/Fetch/apiUpload';
export { apiDownload } from '@/app-ui/Fetch/apiDownload';
export { getAccessToken, setAccessToken } from '@/app-ui/Fetch/accessToken';

export type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';
export type { ApiSuccess } from '@/app-ui/Fetch/types/ApiSuccess';
export type { ApiError } from '@/app-ui/Fetch/types/ApiError';
export type { FetchOptions } from '@/app-ui/Fetch/types/FetchOptions';
export type { UploadProgress } from '@/app-ui/Fetch/types/UploadProgress';
export type { DownloadResult } from '@/app-ui/Fetch/types/DownloadResult';
