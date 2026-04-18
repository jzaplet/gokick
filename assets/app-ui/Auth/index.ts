// Public entry point for the Auth layer.
// Views use authFetch for every protected API call — it wraps apiFetch with
// transparent 401 → refresh → retry. useAuth returns reactive session state
// and the login/logout/refresh actions; permission helpers are re-exported
// for use outside of <script setup>.

export { useAuth } from '@/app-ui/Auth/useAuth';
export { authFetch } from '@/app-ui/Auth/authFetch';
export { login } from '@/app-ui/Auth/login';
export { logout } from '@/app-ui/Auth/logout';
export { refresh } from '@/app-ui/Auth/refresh';
export { clearAuth, isAuthenticated, user } from '@/app-ui/Auth/state';
export {
    hasAllPermissions,
    hasAnyPermission,
    hasPermission,
    hasRole,
    isAdmin,
} from '@/app-ui/Auth/permissions';

export type { AuthUser } from '@/app-ui/Auth/types/AuthUser';
export type { AuthError } from '@/app-ui/Auth/types/AuthError';
export type { LoginRequest } from '@/app-ui/Auth/types/LoginRequest';
export type { LoginResponse } from '@/app-ui/Auth/types/LoginResponse';
