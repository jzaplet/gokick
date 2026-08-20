<script setup lang="ts">
import type { FieldSize } from '@/app-ui/Inputs/field';
import { onMounted, ref } from 'vue';
import { targetValue } from '@/app-ui/Events/eventTarget';
import { fieldBorderClass, fieldId, fieldLabelClass, fieldSizeClass, useFieldValueSync } from '@/app-ui/Inputs/field';

type Props = {
    modelValue?: null | string | number;
    defaultValue?: string;
    type?: string;
    placeholder?: string;
    label?: string;
    // null and undefined both mean "no error" — null so tm(errors.x) binds
    // directly (it returns string | null).
    error?: string | null;
    required?: boolean;
    disabled?: boolean;
    name?: string;
    isNumber?: boolean;
    isNullable?: boolean;
    statusMessage?: string;
    statusVariant?: 'success' | 'info';
    size?: FieldSize;
    withSpinner?: boolean;
    // flat drops the drop shadow — filter panels want chrome-less inputs.
    flat?: boolean;
    // active marks a non-empty filter with an orange border (aibobr parity).
    active?: boolean;
};

const props = defineProps<Props>();
const emit = defineEmits<{
    'update:modelValue': [Props['modelValue']];
    'change': [Props['modelValue']];
    'keyup': [KeyboardEvent];
}>();

const inputId = fieldId(props.name, 'input');

const inputValue = ref(props.defaultValue ?? '');

const resolveValue = (value: Props['modelValue']): string | number | null => {
    if (
        props.isNullable
        && (value === null || value === undefined || value === '')
    ) {
        return null;
    }

    if (
        props.isNumber
        && value !== null
        && value !== undefined
        && value !== ''
    ) {
        const numValue = Number(value);

        return Number.isNaN(numValue) ? value : numValue;
    }

    return value ?? '';
};

const handleInput = (event: Event): void => {
    inputValue.value = targetValue(event);
    const resolvedValue = resolveValue(inputValue.value);

    emit('update:modelValue', resolvedValue);
};

const handleChange = (): void => {
    const resolvedValue = resolveValue(inputValue.value);

    emit('update:modelValue', resolvedValue);
    emit('change', resolvedValue);
};

useFieldValueSync(() => props.modelValue, inputValue);

onMounted(() => {
    emit('update:modelValue', resolveValue(props.modelValue));
});
</script>

<template>
    <div class="space-y-2">
        <label
            v-if="label"
            :for="inputId"
            :class="fieldLabelClass(size)"
        >
            {{ label }}
            <span
                v-if="required"
                class="text-red-500"
            >*</span>
        </label>
        <input
            :id="inputId"
            :name="name"
            :value="inputValue"
            :type="type || 'text'"
            :placeholder="placeholder"
            :disabled="disabled"
            class="w-full border rounded-lg bg-white
        transition-colors focus:outline-none focus:ring-2
        focus:ring-orange-500 focus:border-orange-500"
            :class="[
                props.flat === true ? '' : 'shadow-sm',
                fieldSizeClass(size),
                withSpinner && (size === 'xl' || size === 'lg')
                    ? 'pr-12'
                    : '',
                fieldBorderClass(error, active),
                disabled && 'bg-gray-50 cursor-not-allowed',
            ]"
            @input="handleInput"
            @change="handleChange"
            @keyup="$emit('keyup', $event)"
        >
        <p
            v-if="error"
            class="text-sm text-red-600"
        >
            {{ error }}
        </p>
        <p
            v-if="statusMessage && !error"
            class="text-sm"
            :class="[
                statusVariant === 'success'
                    ? 'text-green-600'
                    : 'text-blue-600',
            ]"
        >
            {{ statusMessage }}
        </p>
    </div>
</template>
