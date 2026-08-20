import type { ApiMessage } from '@/app-ui/Fetch/types/ApiMessage';

export type ChangePasswordErrors = {
    general?: ApiMessage;
    new_password?: ApiMessage;
};
