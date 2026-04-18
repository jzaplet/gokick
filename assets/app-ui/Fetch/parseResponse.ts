import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';

// Converts a raw fetch Response into the discriminated-union ApiResponse.
// Pure: depends only on its Response argument.
export const parseResponse = async <TData, TError>(
    response: Response,
): Promise<ApiResponse<TData, TError>> => {
    let json: unknown = null;

    try {
        json = await response.json();
    } catch {
        // Response has no JSON body.
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
