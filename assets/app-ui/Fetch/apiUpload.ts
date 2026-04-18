import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';
import type { UploadProgress } from '@/app-ui/Fetch/types/UploadProgress';
import { buildAuthHeaders } from '@/app-ui/Fetch/buildHeaders';
import { parseResponse } from '@/app-ui/Fetch/parseResponse';

// multipart/form-data upload via XMLHttpRequest — fetch() has no progress API
// yet (as of 2026), so XHR is still the pragmatic choice when progress is
// needed. Browser sets the Content-Type boundary automatically.
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

        for (const [key, value] of Object.entries(buildAuthHeaders())) {
            xhr.setRequestHeader(key, value);
        }

        xhr.send(formData);
    });
};
