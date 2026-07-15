import type { ApiError } from '@/app-ui/Fetch/types/ApiError';
import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';
import type { Guard } from '@/app-ui/Fetch/guards';
import { isRecord } from '@/app-ui/Fetch/guards';
import { reportUnexpected } from '@/app-ui/Sentry/reportUnexpected';

// A failure with no usable API error body — synthesized in the mergeable
// ApiGeneralError shape (see its doc), NOT pretending to be TError. Exported
// so every synthesis site (network errors in apiFetchCore, XHR failures in
// apiUpload) builds the same shape with the same wording.
export const generalFailure = <TError>(status: number, general: string): ApiError<TError> => ({
    success: false,
    status,
    data: { general },
});

// Sentry groups captureMessage by the message text — a raw URL containing a
// UUID path param would explode one contract bug into a separate issue per
// entity. Normalize before reporting (IDs are always UUIDs in this project).
const uuidRe = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi;

const groupableUrl = (url: string): string => {
    if (url === '') {
        return 'unknown url';
    }
    const path = url.split('?')[0] ?? url;

    return path.replace(uuidRe, ':id');
};

// Converts a raw fetch Response into the discriminated-union ApiResponse.
// Never throws. With a `validate` guard (the tsgen-generated one for TData),
// a 2xx body is validated at runtime: a contract-violating body becomes a
// synthesized { general } failure AND is reported to Sentry — the backend
// provably sent an annotated shape (gk boundary + tsgen), so a mismatch here
// is an unexpected bug (wrong URL↔type pairing on the call site, a middlebox,
// version skew), not a handled 4xx. Without a guard (TData = null, 204
// endpoints) a 2xx body is not inspected and data is null. Failures with no
// usable API error body come back as { general: ... } — mergeable into every
// form's *Errors shape (see ApiError).
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
            return generalFailure<TError>(
                response.status,
                `Malformed response body (status ${String(response.status)})`,
            );
        }

        if (validate !== undefined) {
            if (validate(json)) {
                return { success: true, status: response.status, data: json };
            }
            reportUnexpected(
                `Response shape violates its generated contract: ${groupableUrl(response.url)} (status ${String(response.status)})`,
            );

            return generalFailure<TError>(response.status, 'Invalid response shape');
        }

        // No guard = a TData `null` endpoint (204/no-content): nothing to
        // validate, nothing to hand out.
        return { success: true, status: response.status, data: null as TData };
    }

    if (isRecord(json)) {
        // A real JSON error OBJECT, declared (not validated) as the endpoint's
        // TError — field-key parity is `gk errfields` (follow-up ④).
        return { success: false, status: response.status, data: json as TError };
    }

    // Empty, unparseable, or non-object JSON error body (a proxy answering
    // with a bare string/number would otherwise land in errors.value as a
    // primitive and render NOWHERE) — synthesize the mergeable { general }
    // shape; keep a bare-string body as the message since it is human-readable.
    if (typeof json === 'string' && json !== '') {
        return generalFailure<TError>(response.status, json);
    }

    return generalFailure<TError>(response.status, `Error ${String(response.status)}`);
};
