<script setup lang="ts">
import type { FieldSize } from '@/app-ui/Inputs/field';
import { ref } from 'vue';
import { targetValue } from '@/app-ui/Events/eventTarget';
import { fieldBorderClass, fieldId, fieldLabelClass, fieldSizeClass, useFieldValueSync } from '@/app-ui/Inputs/field';

type Option = {
    value: string;
    label: string;
};

type Props = {
    modelValue?: string | null;
    options: Option[];
    placeholder?: string;
    label?: string;
    // null and undefined both mean "no error" — null so tm(errors.x) binds
    // directly (it returns string | null).
    error?: string | null;
    required?: boolean;
    disabled?: boolean;
    name?: string;
    size?: FieldSize;
    // flat drops the drop shadow — filter panels want chrome-less inputs.
    flat?: boolean;
    // active marks a non-empty filter with an orange border (aibobr parity).
    active?: boolean;
};

const props = defineProps<Props>();
const emit = defineEmits<{
    'update:modelValue': [string | null];
    'change': [string | null];
}>();

const inputId = fieldId(props.name, 'select');

const selectValue = ref(props.modelValue ?? '');

const handleChange = (event: Event): void => {
    const value = targetValue(event);

    selectValue.value = value;
    const emitted = value === '' ? null : value;

    emit('update:modelValue', emitted);
    emit('change', emitted);
};

useFieldValueSync(() => props.modelValue, selectValue);
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
        <select
            :id="inputId"
            :name="name"
            :value="selectValue"
            :disabled="disabled"
            class="w-full border rounded-lg bg-white
        transition-colors focus:outline-none focus:ring-2
        focus:ring-orange-500 focus:border-orange-500
        appearance-none cursor-pointer"
            :class="[
                props.flat === true ? '' : 'shadow-sm',
                fieldSizeClass(size),
                fieldBorderClass(error, active),
                disabled && 'bg-gray-50 cursor-not-allowed',
            ]"
            @change="handleChange"
        >
            <option
                v-if="placeholder"
                value=""
                disabled
            >
                {{ placeholder }}
            </option>
            <option
                v-for="option in options"
                :key="option.value"
                :value="option.value"
            >
                {{ option.label }}
            </option>
        </select>
        <p
            v-if="error"
            class="text-sm text-red-600"
        >
            {{ error }}
        </p>
    </div>
</template>

<style scoped>
select {
    background-image: url("data:image/svg+xml,%3csvg xmlns='http://www.w3.org/2000/svg' fill='none' viewBox='0 0 20 20'%3e%3cpath stroke='%236b7280' stroke-linecap='round' stroke-linejoin='round' stroke-width='1.5' d='M6 8l4 4 4-4'/%3e%3c/svg%3e");
    background-position: right 0.5rem center;
    background-repeat: no-repeat;
    background-size: 1.5em 1.5em;
    padding-right: 2.5rem;
}
</style>
