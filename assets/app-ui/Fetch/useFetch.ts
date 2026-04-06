import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';
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
    url: string,
    options: FetchOptions = {},
): Promise<ApiResponse<TData, TError>> => {
    const headers = buildHeaders({
        'Content-Type': 'application/json',
        ...options.headers,
    });

    const init: RequestInit = {
        method: options.method ?? 'GET',
        headers,
        credentials: 'same-origin',
    };

    if (options.body !== undefined) {
        init.body = JSON.stringify(options.body);
    }

    const response = await fetch(`/api/v1${url}`, init);

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

        xhr.open('POST', `/api/v1${url}`);

        const headers = buildHeaders();

        for (const [key, value] of Object.entries(headers)) {
            xhr.setRequestHeader(key, value);
        }

        // No Content-Type — browser sets multipart/form-data with boundary
        xhr.send(formData);
    });
};
