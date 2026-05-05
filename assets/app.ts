import { createApp } from 'vue';
import App from '@/App.vue';
import { router } from '@/router';
import { refresh } from '@/app-ui/Auth/refresh';
import '@/tailwind.css';
import '@/img/go-vue-cqrs-ddd.png';

// Access token žije jen v paměti (XSS-rezistentní), takže ho hard refresh
// stránky vždy vymaže. Refresh token sedí v HttpOnly cookie a přežije —
// zkusíme tedy obnovit session ze cookie ještě než se mount router-guard.
// Pokud cookie chybí nebo je neplatná, refresh tiše selže a guard pošle
// chráněné routy na /login (jako u úplně nového návštěvníka).
const bootstrap = async (): Promise<void> => {
    await refresh();

    createApp(App).use(router).mount('#app');
};

void bootstrap();
