import type { App } from 'vue';
import * as Sentry from '@sentry/vue';
import { sentryDsn, sentryEnvironment, sentryRelease } from '@/app-ui/Sentry/runtimeConfig';
import { syncSentryUser } from '@/app-ui/Sentry/syncUser';

// Error & panic capture (scope A) — no session replay, and crucially **no
// performance data is sent**. Gated on the resolved DSN (a runtime <meta> tag
// injected by the Go server, or build-time VITE_SENTRY_DSN under the Vite dev
// server): without it Sentry never initializes, so the app runs unchanged and
// ships no telemetry.
//
// The default browser integrations capture uncaught errors, unhandled promise
// rejections and Vue component errors. Ordinary handled API errors (4xx from
// authFetch/apiFetch) are NOT reported here — only unexpected failures.
//
// browserTracing + tracesSampleRate:0 is "distributed tracing without
// performance": the SDK propagates the trace (sentry-trace / baggage headers) to
// the backend on same-origin /api calls — the default tracePropagationTargets is
// same-origin, so nothing leaks to the Sentry ingest — so a frontend error and
// the backend error it triggers share one trace id and link in Sentry. Rate 0
// means NO transactions are sent (no performance, no quota). Full performance
// tracing (spans, waterfall) is roadmap scope B.
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
        integrations: [Sentry.browserTracingIntegration()],
        tracesSampleRate: 0,
    });

    // Attribute every event to the current user, kept in sync with the session.
    syncSentryUser();
};
