import { createApp } from 'vue';
import App from '@/App.vue';
import { router } from '@/router';
import { refresh } from '@/app-ui/Auth/refresh';
import '@/tailwind.css';
import '@/img/go-vue-cqrs-ddd.png';

// The access token lives only in memory (XSS-resistant), so a hard page
// refresh always wipes it. The refresh token sits in an HttpOnly cookie and
// survives — we therefore attempt to restore the session from the cookie
// before mounting the router guard. If the cookie is missing or invalid,
// refresh fails silently and the guard sends protected routes to /login
// (just like a brand-new visitor).
const bootstrap = async (): Promise<void> => {
    await refresh();

    createApp(App).use(router).mount('#app');
};

void bootstrap();
