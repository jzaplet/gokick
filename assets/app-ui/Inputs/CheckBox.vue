<script setup lang="ts">
// Minimal styled native checkbox — native input keeps keyboard and screen-
// reader behavior for free (the aibobr original rebuilt it on a button).
const { modelValue, disabled, srLabel } = defineProps<{
    modelValue: boolean;
    disabled?: boolean;
    // Screen-reader label — named srLabel (not ariaLabel) so the template
    // attribute sr-label cannot collide with the native aria-label attr.
    srLabel: string;
}>();

const emit = defineEmits<{
    'update:modelValue': [value: boolean];
}>();

const onChange = (event: Event): void => {
    if (event.target instanceof HTMLInputElement) {
        emit('update:modelValue', event.target.checked);
    }
};
</script>

<template>
    <input
        type="checkbox"
        :checked="modelValue"
        :disabled="disabled"
        :aria-label="srLabel"
        :class="[
            'w-4 h-4 rounded',
            'border-gray-300 text-orange-500',
            'focus:ring-orange-500',
            'cursor-pointer disabled:cursor-not-allowed disabled:opacity-40',
        ]"
        @change="onChange"
    >
</template>
