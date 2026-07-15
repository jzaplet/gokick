import type { Ref } from 'vue';
import { watch } from 'vue';

// Shared form-field plumbing for the Inputs family (F-086): Input and Select
// carried verbatim copies of the size-class ternary, the fallback-id generation
// and the modelValue→display watch-sync; a fix landing in only one copy was the
// drift risk. One source here, both components consume it.

export type FieldSize = 'sm' | 'md' | 'lg' | 'xl';

export const fieldSizeClass = (size: FieldSize | undefined): string => {
    if (size === 'xl') {
        return 'px-6 py-4 text-lg';
    }
    if (size === 'lg') {
        return 'px-4 py-3 text-base';
    }
    if (size === 'sm') {
        return 'px-2 py-1 text-sm';
    }

    return 'px-3 py-2';
};

// Stable id for the label/for pairing: the field name when given, otherwise a
// one-shot random id with a per-component prefix.
export const fieldId = (name: string | undefined, prefix: string): string =>
    name ?? `${prefix}-${Math.random().toString(36).substring(2, 9)}`;

// Keeps the internal display ref in sync with the (possibly null/undefined)
// modelValue prop: empty string for absent values, String() otherwise.
export const useFieldValueSync = (
    source: () => string | number | null | undefined,
    target: Ref<string>,
): void => {
    watch(
        source,
        (newValue) => {
            if (newValue === null || newValue === undefined) {
                target.value = '';
            } else {
                target.value = String(newValue);
            }
        },
        { immediate: true },
    );
};
