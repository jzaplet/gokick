import type { ApiMessage } from '@/app-ui/Fetch/types/ApiMessage';

// The shape the fetch layer SYNTHESIZES for failures that carry no API error
// body: network errors (status 0), malformed 2xx bodies, contract violations
// (a 2xx failing its generated guard) and error statuses with an unusable body.
// `general` is deliberately the SAME key the backend uses for non-field errors
// (Responder.Error → { "general": { key, params } }), and it carries the same
// wire-shaped ApiMessage — a catalog key + params rendered at the display site
// via tm(), exactly like a real backend error. Every form's *Errors type has
// `general?: ApiMessage`, so `errors.value = result.data` stays a one-line
// merge with no narrowing, and a network error surfaces in the form's general
// slot in the active language.
export type ApiGeneralError = {
    general: ApiMessage;
};
