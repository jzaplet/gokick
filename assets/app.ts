import { createApp } from 'vue';
import App from '@/App.vue';
import { router } from '@/router';
import { refresh } from '@/app-ui/Auth/refresh';
import { hasSessionHint } from '@/app-ui/Auth/sessionHint';
import { initSentry } from '@/app-ui/Sentry/initSentry';
import '@/tailwind.css';
import '@/img/go-vue-cqrs-ddd.png';

// The access token lives only in memory (XSS-resistant), so a hard page
// refresh always wipes it. The refresh token sits in an HttpOnly cookie and
// survives — we therefore attempt to restore the session from the cookie
// before mounting the router guard. If the cookie is missing or invalid,
// refresh fails silently and the guard sends protected routes to /login
// (just like a brand-new visitor).
export const bootstrap = async (): Promise<void> => {
    const app = createApp(App);

    // Init error tracking before the first await so failures during refresh and
    // mount are captured too (no-op without VITE_SENTRY_DSN).
    initSentry(app);

    // Only attempt the cookie-based restore when the readable hint says a
    // session plausibly exists — a guest skips it (and its guaranteed 401).
    // Defensive try/catch: refresh() is written to never throw, but bootstrap
    // MUST mount the app no matter what — a future regression (or any unexpected
    // throw) degrades to the guest path instead of leaving a blank page.
    try {
        if (hasSessionHint() === true) {
            await refresh();
        }
    } catch {
        // Ignore — fall through to mount as a guest.
    }

    app.use(router).mount('#app');
};

void bootstrap();
