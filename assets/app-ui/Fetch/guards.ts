// Runtime guard primitives for the wire boundary. tsgen-generated guards
// (export const isX = (v): v is X => ...) compose these to validate a parsed
// JSON body against the generated type — the runtime half of the parity loop
// (the compile-time half is the generated type itself).
export type Guard<T> = (v: unknown) => v is T;

export const isRecord = (v: unknown): v is Record<string, unknown> =>
    typeof v === 'object' && v !== null && Array.isArray(v) === false;

export const isString = (v: unknown): v is string => typeof v === 'string';

export const isNumber = (v: unknown): v is number => typeof v === 'number';

export const isBoolean = (v: unknown): v is boolean => typeof v === 'boolean';

export const arrayOf = <T>(guard: Guard<T>): Guard<T[]> =>
    (v: unknown): v is T[] => Array.isArray(v) && v.every((e: unknown) => guard(e));

export const nullable = <T>(guard: Guard<T>): Guard<T | null> =>
    (v: unknown): v is T | null => v === null || guard(v);

// NOTE: tsgen emits `optional(...)` for an ,omitempty DTO field. No annotated
// DTO carries one today, so the helper does not exist yet (knip would flag a
// dead export) — add it here the day the first omitempty field appears:
//   export const optional = <T>(guard: Guard<T>): Guard<T | undefined> =>
//       (v: unknown): v is T | undefined => v === undefined || guard(v);
