import type { AuthUser } from '@/app-ui/Auth/types/AuthUser';

export type LoginResponse = {
    access_token: string;
    access_expiration: number;
    user: AuthUser;
};
