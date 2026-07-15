// TBody defaults to `never`: a call that passes `body` without declaring its
// request type simply does not compile — nothing is assignable to `never`.
// NoInfer keeps that promise at the COMPILER level: without it, TypeScript
// would infer TBody from the body argument itself and the `never` default
// would never kick in — the ESLint explicit-generics rule would be the only
// gate, and a name-based lint rule misses aliased/namespace-imported calls.
// Declare the third generic with the tsgen-GENERATED request type
// (e.g. authFetch<null, UserFormErrors, UserFormData>) so the payload is
// structurally checked against the same shape the Go handler decodes.
// This is the FE half of the wire boundary; the BE half is `gk boundary`.
export type FetchOptions<TBody = never> = {
    body?: NoInfer<TBody>;
    headers?: Record<string, string>;
};
