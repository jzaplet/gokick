import type { ApiMessage } from '@/app-ui/Fetch/types/ApiMessage';

// Default auth error shape. Backend field-keys errors:
//   - non-field errors    → { "general": { key, params } }
//   - validation errors   → { "nickname": { key, params } } (or other field)
// Values are ApiMessage wire messages, rendered via tm() at display time.
// Forms extend this shape with their own known fields and pass it to
// apiFetch / login as the TError generic.
export type AuthError = {
    general?: ApiMessage;
};
