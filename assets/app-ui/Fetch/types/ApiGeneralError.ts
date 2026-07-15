// The shape the fetch layer SYNTHESIZES for failures that carry no API error
// body: network errors (status 0), malformed 2xx bodies, contract violations
// (a 2xx failing its generated guard) and error statuses with an empty body.
// `general` is deliberately the SAME key the backend uses for non-field errors
// (Responder.Error → { "general": "..." }), and every form's *Errors type has
// `general?: string` — so `errors.value = result.data` stays a one-line merge
// with no narrowing, and a network error surfaces in the form's general slot.
export type ApiGeneralError = {
    general: string;
};
