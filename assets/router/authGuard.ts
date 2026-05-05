import type { NavigationGuard } from 'vue-router';
import { hasPermission, isAuthenticated } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';

// Navigation guard that enforces `meta.requiresAuth` and `meta.requiresPermission`.
// Both fields come from AppRoute (see meta.ts) and are statically required by
// TypeScript, so no runtime fallback is needed — a missing meta.requiresAuth
// is a compile-time error.
export const authGuard: NavigationGuard = (to) => {
    const { error, info } = useToast();

    if (to.meta.requiresAuth === true && isAuthenticated.value === false) {
        info('Please sign in to continue.');

        return {
            name: 'login',
            query: to.fullPath === '/' ? {} : { redirect: to.fullPath },
        };
    }

    if (to.meta.requiresPermission !== undefined
        && hasPermission(to.meta.requiresPermission) === false) {
        error('You do not have permission to access this page.');

        return { name: 'home' };
    }

    return true;
};
