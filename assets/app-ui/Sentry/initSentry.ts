import type { App } from 'vue';
import * as Sentry from '@sentry/vue';

// Error & panic capture only (scope A) — no performance tracing or session
// replay. Gated on VITE_SENTRY_DSN: without it Sentry never initializes, so the
// app runs unchanged and ships no telemetry (safe to build/deploy with no DSN).
//
// The default browser integrations capture uncaught errors, unhandled promise
// rejections and Vue component errors. Ordinary handled API errors (4xx from
// authFetch/apiFetch) are NOT reported here — only unexpected failures.
export const initSentry = (app: App): void => {
    const dsn = import.meta.env.VITE_SENTRY_DSN;

    if (dsn === undefined || dsn === '') {
        return;
    }

    // Baked in at build time from the git tag (Makefile / Dockerfile). An empty
    // string would create a bogus empty-named release, so map it to undefined.
    const release = import.meta.env.VITE_SENTRY_RELEASE;

    Sentry.init({
        app,
        dsn,
        environment: import.meta.env.VITE_SENTRY_ENVIRONMENT ?? 'development',
        release: release === '' ? undefined : release,
    });
};
