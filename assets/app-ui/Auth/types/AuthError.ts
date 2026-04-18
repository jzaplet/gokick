// Default auth error shape. Backend field-keys errors:
//   - non-field errors    → { "general": "..." }
//   - validation errors   → { "nickname": "..." } (or other field)
// Forms extend this shape with their own known fields and pass it to
// apiFetch / login as the TError generic.
export type AuthError = {
    general?: string;
};
