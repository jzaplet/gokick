import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';
import type { Guard } from '@/app-ui/Fetch/guards';
import type { UploadProgress } from '@/app-ui/Fetch/types/UploadProgress';
import { buildAuthHeaders } from '@/app-ui/Fetch/buildHeaders';
import { parseResponse } from '@/app-ui/Fetch/parseResponse';

type UploadOptions<TData> = {
    // The upload's JSON response takes part in the parity loop like any other:
    // validate is the tsgen-generated guard for TData. (The multipart REQUEST
    // half is outside the JSON loop by nature — files, not a DTO.)
    validate: Guard<TData>;
    onProgress?: (stats: UploadProgress) => void;
};

// multipart/form-data upload via XMLHttpRequest — fetch() has no progress API
// yet (as of 2026), so XHR is still the pragmatic choice when progress is
// needed. Browser sets the Content-Type boundary automatically.
export const apiUpload = async <TData, TError = { message: string }>(
    url: string,
    formData: FormData,
    options: UploadOptions<TData>,
): Promise<ApiResponse<TData, TError>> => {
    return new Promise((resolve) => {
        const xhr = new XMLHttpRequest();

        if (options.onProgress !== undefined) {
            const report = options.onProgress;

            xhr.upload.onprogress = (event: ProgressEvent): void => {
                if (event.lengthComputable) {
                    report({
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

            void parseResponse<TData, TError>(response, options.validate).then(resolve);
        };

        xhr.onerror = (): void => {
            resolve({
                success: false,
                transport: true,
                status: xhr.status,
                message: xhr.responseText || 'Network error',
            });
        };

        xhr.open('POST', url);

        for (const [key, value] of Object.entries(buildAuthHeaders())) {
            xhr.setRequestHeader(key, value);
        }

        xhr.send(formData);
    });
};
