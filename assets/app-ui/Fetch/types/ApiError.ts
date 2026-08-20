import type { ApiGeneralError } from '@/app-ui/Fetch/types/ApiGeneralError';

// A failed response. `data` is the endpoint's declared TError when the API
// sent a JSON error body, or the synthesized ApiGeneralError when the failure
// happened below the API (network / malformed body / contract violation —
// status 0 marks "never reached the server"). The union is what makes the
// synthesis SOUND (no `as TError` lie) while keeping the one-line merge:
// every *Errors type carries `general?: ApiMessage`, so both arms assign to it.
export type ApiError<TError> = {
    success: false;
    status: number;
    data: TError | ApiGeneralError;
};
