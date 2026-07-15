import * as Sentry from '@sentry/vue';

// The ONE manual FE report entry point, mirroring the backend rule: error
// reporting is for the UNEXPECTED only (a response that violates its generated
// contract, an invariant break) — never for handled API 4xx. captureMessage is
// a safe no-op when initSentry never ran (no DSN), so callers don't gate.
export const reportUnexpected = (message: string): void => {
    Sentry.captureMessage(message, 'error');
};
