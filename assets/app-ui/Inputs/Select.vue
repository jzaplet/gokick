<script setup lang="ts">
import { ref, watch } from 'vue';

type Option = {
    value: string;
    label: string;
};

type Props = {
    modelValue?: string | null;
    options: Option[];
    placeholder?: string;
    label?: string;
    error?: string | undefined;
    required?: boolean;
    disabled?: boolean;
    name?: string;
    size?: 'sm' | 'md' | 'lg' | 'xl';
};

const props = defineProps<Props>();
const emit = defineEmits<{
    'update:modelValue': [string | null];
    'change': [string | null];
}>();

const inputId
    = props.name ?? `select-${Math.random().toString(36).substring(2, 9)}`;

const selectValue = ref(props.modelValue ?? '');

const handleChange = (event: Event): void => {
    const target = event.target as HTMLSelectElement;

    selectValue.value = target.value;
    const value = target.value === '' ? null : target.value;

    emit('update:modelValue', value);
    emit('change', value);
};

watch(
    () => props.modelValue,
    (newValue) => {
        if (newValue === null || newValue === undefined) {
            selectValue.value = '';
        } else {
            selectValue.value = newValue;
        }
    },
    { immediate: true },
);
</script>

<template>
    <div class="space-y-2">
        <label
            v-if="label"
            :for="inputId"
            class="block text-sm font-medium text-gray-700"
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
            class="w-full border rounded-lg shadow-sm bg-white
        transition-colors focus:outline-none focus:ring-2
        focus:ring-orange-500 focus:border-orange-500
        appearance-none cursor-pointer"
            :class="[
                size === 'xl'
                    ? 'px-6 py-4 text-lg'
                    : size === 'lg'
                        ? 'px-4 py-3 text-base'
                        : size === 'sm'
                            ? 'px-2 py-1 text-sm'
                            : 'px-3 py-2',
                error
                    ? 'border-red-300 focus:ring-red-500'
                    : 'border-gray-300 hover:border-gray-400',
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
