import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';
import type { ApiTransport } from '@/app-ui/Fetch/types/ApiTransport';
import type { Guard } from '@/app-ui/Fetch/guards';
import { reportUnexpected } from '@/app-ui/Sentry/reportUnexpected';

const transport = (status: number, message: string): ApiTransport => ({
    success: false,
    transport: true,
    status,
    message,
});

// Converts a raw fetch Response into the discriminated-union ApiResponse.
// Never throws. With a `validate` guard (the tsgen-generated one for TData),
// a 2xx body is validated at runtime: a contract-violating body becomes an
// ApiTransport failure AND is reported to Sentry — the backend provably sent
// that shape (gk boundary + tsgen), so a mismatch here is an unexpected bug
// (wrong URL↔type pairing on the call site, a middlebox, version skew), not
// a handled 4xx. Without a guard (TData = null, 204 endpoints) a 2xx body is
// not inspected and data is null.
export const parseResponse = async <TData, TError>(
    response: Response,
    validate?: Guard<TData>,
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
            return transport(
                response.status,
                `Malformed response body (status ${String(response.status)})`,
            );
        }

        if (validate !== undefined) {
            if (validate(json)) {
                return { success: true, status: response.status, data: json };
            }
            reportUnexpected(
                `Response shape violates its generated contract: ${response.url === '' ? 'unknown url' : response.url} (status ${String(response.status)})`,
            );

            return transport(response.status, 'Invalid response shape');
        }

        // No guard = a TData `null` endpoint (204/no-content): nothing to
        // validate, nothing to hand out.
        return { success: true, status: response.status, data: null as TData };
    }

    if (json === null) {
        // An error status with an empty/unparseable body has no TError to carry.
        return transport(response.status, `Error ${String(response.status)}`);
    }

    // A real JSON error body, declared (not validated) as the endpoint's TError —
    // field-key parity is roadmap follow-up ④.
    return { success: false, status: response.status, data: json as TError };
};
