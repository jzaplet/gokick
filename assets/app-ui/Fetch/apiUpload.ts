import type { ApiGeneralError } from '@/app-ui/Fetch/types/ApiGeneralError';
import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';
import type { Guard } from '@/app-ui/Fetch/guards';
import type { UploadProgress } from '@/app-ui/Fetch/types/UploadProgress';
import { buildAuthHeaders } from '@/app-ui/Fetch/buildHeaders';
import { generalFailure, parseResponse } from '@/app-ui/Fetch/parseResponse';

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
export const apiUpload = async <TData, TError = ApiGeneralError>(
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
            // A null-body status (204/205/304) REJECTS a non-null body — even ''
            // — with a TypeError, and a throw inside an XHR event handler never
            // reaches this promise: it would stay pending forever. Hand null to
            // the Response instead; parseResponse already treats an empty body
            // as a legitimate bodyless success.
            const response = new Response(
                xhr.responseText === '' ? null : xhr.responseText,
                {
                    status: xhr.status,
                    statusText: xhr.statusText,
                },
            );

            void parseResponse<TData, TError>(response, options.validate).then(resolve);
        };

        xhr.onerror = (): void => {
            resolve(generalFailure<TError>(xhr.status, 'Network error'));
        };

        xhr.open('POST', url);

        for (const [key, value] of Object.entries(buildAuthHeaders())) {
            xhr.setRequestHeader(key, value);
        }

        xhr.send(formData);
    });
};
