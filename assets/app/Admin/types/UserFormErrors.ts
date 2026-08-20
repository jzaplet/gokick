import type { ApiMessage } from '@/app-ui/Fetch/types/ApiMessage';

export type UserFormErrors = {
    general?: ApiMessage;
    nickname?: ApiMessage;
    password?: ApiMessage;
    email?: ApiMessage;
    role?: ApiMessage;
};
