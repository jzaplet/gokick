import type { App } from 'vue';
import * as Sentry from '@sentry/vue';
import { sentryDsn, sentryEnvironment, sentryRelease } from '@/app-ui/Sentry/runtimeConfig';

// Error & panic capture only (scope A) — no performance tracing or session
// replay. Gated on the resolved DSN (a runtime <meta> tag injected by the Go
// server, or build-time VITE_SENTRY_DSN under the Vite dev server): without it
// Sentry never initializes, so the app runs unchanged and ships no telemetry.
//
// The default browser integrations capture uncaught errors, unhandled promise
// rejections and Vue component errors. Ordinary handled API errors (4xx from
// authFetch/apiFetch) are NOT reported here — only unexpected failures.
export const initSentry = (app: App): void => {
    const dsn = sentryDsn();

    if (dsn === undefined || dsn === '') {
        return;
    }

    Sentry.init({
        app,
        dsn,
        environment: sentryEnvironment(),
        release: sentryRelease(),
    });
};
