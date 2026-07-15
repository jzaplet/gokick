// TBody defaults to `never`: a call that passes `body` without declaring its
// request type simply does not compile — nothing is assignable to `never`.
// Declare the third generic with the tsgen-GENERATED request type
// (e.g. authFetch<null, UserFormErrors, UserFormData>) so the payload is
// structurally checked against the same shape the Go handler decodes.
// This is the FE half of the wire boundary; the BE half is `gk boundary`.
export type FetchOptions<TBody = never> = {
    body?: TBody;
    headers?: Record<string, string>;
};
