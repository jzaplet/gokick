import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';
import type { DownloadResult } from '@/app-ui/Fetch/types/DownloadResult';
import type { FetchOptions } from '@/app-ui/Fetch/types/FetchOptions';
import type { UploadProgress } from '@/app-ui/Fetch/types/UploadProgress';

let accessToken: string | null = null;

export const setAccessToken = (token: string | null): void => {
    accessToken = token;
};

export const getAccessToken = (): string | null => {
    return accessToken;
};

const buildHeaders = (extra: Record<string, string> = {}): Record<string, string> => {
    const headers: Record<string, string> = { ...extra };

    if (accessToken !== null) {
        headers['Authorization'] = `Bearer ${accessToken}`;
    }

    return headers;
};

const parseResponse = async <TData, TError>(
    response: Response,
): Promise<ApiResponse<TData, TError>> => {
    let json: unknown = null;

    try {
        json = await response.json();
    } catch {
        // Response has no JSON body
    }

    if (response.ok) {
        return {
            success: true,
            status: response.status,
            data: json as TData,
        };
    }

    return {
        success: false,
        status: response.status,
        data: (json ?? { message: `Error ${String(response.status)}` }) as TError,
    };
};

export const apiFetch = async <TData, TError = { message: string }>(
    method: string,
    url: string,
    options: FetchOptions = {},
): Promise<ApiResponse<TData, TError>> => {
    const headers = buildHeaders({
        'Content-Type': 'application/json',
        ...options.headers,
    });

    const init: RequestInit = {
        method,
        headers,
        credentials: 'same-origin',
    };

    if (options.body !== undefined) {
        init.body = JSON.stringify(options.body);
    }

    const response = await fetch(url, init);

    return parseResponse<TData, TError>(response);
};

export const apiUpload = async <TData, TError = { message: string }>(
    url: string,
    formData: FormData,
    onProgress?: (stats: UploadProgress) => void,
): Promise<ApiResponse<TData, TError>> => {
    return new Promise((resolve) => {
        const xhr = new XMLHttpRequest();

        if (onProgress !== undefined) {
            xhr.upload.onprogress = (event: ProgressEvent): void => {
                if (event.lengthComputable) {
                    onProgress({
                        percent: (event.loaded / event.total) * 100,
                        loaded: event.loaded,
                        total: event.total,
                    });
                }
            };
        }

        xhr.onload = (): void => {
            const response = new Response(xhr.responseText, {
                status: xhr.status,
                statusText: xhr.statusText,
            });

            void parseResponse<TData, TError>(response).then(resolve);
        };

        xhr.onerror = (): void => {
            resolve({
                success: false,
                status: xhr.status,
                data: { message: xhr.responseText || 'Network error' } as TError,
            });
        };

        xhr.open('POST', url);

        const headers = buildHeaders();

        for (const [key, value] of Object.entries(headers)) {
            xhr.setRequestHeader(key, value);
        }

        // No Content-Type — browser sets multipart/form-data with boundary
        xhr.send(formData);
    });
};

const parseFilename = (response: Response, fallback: string): string => {
    const disposition = response.headers.get('Content-Disposition');
    const match = disposition?.match(/filename="?(.+?)"?$/);

    if (match !== null && match !== undefined) {
        return match[1] ?? fallback;
    }

    return fallback;
};

const triggerDownload = (blob: Blob, filename: string): void => {
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');

    link.href = url;
    link.download = filename;
    link.click();
    URL.revokeObjectURL(url);
};

export const apiDownload = async (
    url: string,
    fallbackFilename: string,
): Promise<DownloadResult> => {
    const response = await fetch(url, {
        headers: buildHeaders(),
        credentials: 'same-origin',
    });

    if (response.ok === false) {
        return { success: false, status: response.status, filename: null };
    }

    const blob = await response.blob();
    const filename = parseFilename(response, fallbackFilename);

    triggerDownload(blob, filename);

    return { success: true, status: response.status, filename };
};
