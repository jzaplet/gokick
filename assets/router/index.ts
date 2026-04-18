import { createRouter, createWebHistory } from 'vue-router';
import { authGuard } from '@/router/authGuard';
import { routes } from '@/router/routes';

export { authGuard } from '@/router/authGuard';
export type { AppRoute } from '@/router/meta';

export const router = createRouter({
    history: createWebHistory(),
    routes,
});

router.beforeEach(authGuard);
