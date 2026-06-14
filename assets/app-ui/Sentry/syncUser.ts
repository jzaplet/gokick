import { watch } from 'vue';
import * as Sentry from '@sentry/vue';
import { user } from '@/app-ui/Auth/state';

// Keeps the Sentry user in lock-step with the session. Whenever login or a token
// refresh populates the session user — or clearAuth wipes it on logout, refresh
// failure, or a give-up 401 — the change flows through here, so every captured
// event is attributed to the right person (and to nobody once they log out).
// Watching the single session ref means no auth path can forget to update
// Sentry: it's one reaction, not a call duplicated across login/refresh/logout.
//
// Wired from initSentry, so it only runs once Sentry is initialized. immediate
// aligns Sentry with whatever the session already holds at init time, regardless
// of whether the session was restored before or after this is set up.
export const syncSentryUser = (): void => {
    watch(
        user,
        (current) => {
            if (current === null) {
                Sentry.setUser(null);

                return;
            }

            Sentry.setUser({
                id: current.id,
                username: current.nickname,
                email: current.email,
                role: current.role,
            });
        },
        { immediate: true },
    );
};
