import type { ApiError } from '@/app-ui/Fetch/types/ApiError';
import type { ApiResponse } from '@/app-ui/Fetch/types/ApiResponse';
import type { Guard } from '@/app-ui/Fetch/guards';
import { isRecord } from '@/app-ui/Fetch/guards';
import type { TranslationKey, TranslationParams } from '@/app-ui/I18n/lang';
import { reportUnexpected } from '@/app-ui/Sentry/reportUnexpected';

// A failure with no usable API error body — synthesized in the mergeable
// ApiGeneralError shape (see its doc), NOT pretending to be TError. The
// message is a wire-shaped ApiMessage built from a CATALOG key (typed, so a
// typo fails vue-tsc), not a pre-rendered sentence — rendering happens at the
// display site via tm(), the same path real backend errors take, so the text
// follows a language switch. Exported so every synthesis site (network errors
// in apiFetchCore, XHR failures in apiUpload) builds the same shape with the
// same keys.
export const generalFailure = <TError>(
    status: number,
    key: TranslationKey,
    params?: TranslationParams,
): ApiError<TError> => ({
    success: false,
    status,
    data: { general: params === undefined ? { key } : { key, params } },
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
                'fetch.malformed_body',
                { status: response.status },
            );
        }

        if (validate !== undefined) {
            if (validate(json)) {
                return { success: true, status: response.status, data: json };
            }
            reportUnexpected(
                `Response shape violates its generated contract: ${groupableUrl(response.url)} (status ${String(response.status)})`,
            );

            return generalFailure<TError>(response.status, 'fetch.invalid_shape');
        }

        // No guard = a TData `null` endpoint (204/no-content): nothing to
        // validate, nothing to hand out.
        // The cast is irreducible, not laziness: the "no validate ⇔ TData is
        // null" invariant lives in apiFetch's OVERLOADS, which this generic
        // body cannot see. There is also nothing to guard — a 204 has no body.
        // eslint-disable-next-line @typescript-eslint/consistent-type-assertions -- see above
        return { success: true, status: response.status, data: null as TData };
    }

    if (isRecord(json)) {
        // A real JSON error OBJECT, declared (not validated) as the endpoint's
        // TError — field-key parity is `gk errfields` (follow-up ④).
        // The cast is irreducible: TError is caller-supplied, so there is no
        // guard to call. isRecord above pins the one property consumers rely
        // on — it IS an object, mergeable into errors.value — so a primitive
        // never reaches here; only the exact key set is unproven.
        // eslint-disable-next-line @typescript-eslint/consistent-type-assertions -- see above
        return { success: false, status: response.status, data: json as TError };
    }

    // Empty, unparseable, or non-object JSON error body (a proxy answering
    // with a bare string/number would otherwise land in errors.value as a
    // primitive and render NOWHERE) — synthesize the mergeable { general }
    // shape. A bare-string proxy body is NOT kept as the message: the general
    // slot renders catalog keys via tm(), and a raw proxy string can't be one
    // — the status param names the failure instead.
    return generalFailure<TError>(response.status, 'fetch.error_status', { status: response.status });
};
