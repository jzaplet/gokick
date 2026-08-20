import type { ApiMessage } from '@/app-ui/Fetch/types/ApiMessage';

// Error shape for the profile language change (PUT /api/v1/profile/lang).
// Hand-written on purpose (error-shape types are not generated); keys mirror
// the backend ValidationError fields, `general` is the shared catch-all.
// Values are wire messages ({ key, params }) rendered via tm() at display time.
export type ChangeLangErrors = {
    general?: ApiMessage;
    lang?: ApiMessage;
};
