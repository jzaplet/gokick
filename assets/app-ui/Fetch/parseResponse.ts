import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';

// Converts a raw fetch Response into the discriminated-union ApiResponse.
// Pure: depends only on its Response argument, and never throws.
export const parseResponse = async <TData, TError>(
    response: Response,
): Promise<ApiResponse<TData, TError>> => {
    // Read the body as text first so an EMPTY body (204 / no content) is
    // distinguishable from a NON-empty body that fails to parse. The former is a
    // legitimate bodyless success; the latter is a corrupt payload we must NOT
    // swallow into a fake success (F-081) — a 200 with garbage is not a session.
    let text = '';

    try {
        text = await response.text();
    } catch {
        // Interrupted body read — treat as an empty body (same tolerance as a
        // bodyless 2xx); the parse-failure path below only trips on a body read OK.
    }

    let json: unknown = null;
    let parseFailed = false;

    if (text !== '') {
        try {
            json = JSON.parse(text);
        } catch {
            parseFailed = true;
        }
    }

    if (response.ok) {
        if (parseFailed === true) {
            return {
                success: false,
                status: response.status,
                data: {
                    message: `Malformed response body (status ${String(response.status)})`,
                } as TError,
            };
        }

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
