<script setup lang="ts">
import { onMounted, ref, watch } from 'vue';

type Props = {
    modelValue?: null | string | number;
    defaultValue?: string;
    type?: string;
    placeholder?: string;
    label?: string;
    error?: string | undefined;
    required?: boolean;
    disabled?: boolean;
    name?: string;
    isNumber?: boolean;
    isNullable?: boolean;
    statusMessage?: string;
    statusVariant?: 'success' | 'info';
    size?: 'sm' | 'md' | 'lg' | 'xl';
    withSpinner?: boolean;
};

const props = defineProps<Props>();
const emit = defineEmits<{
    'update:modelValue': [Props['modelValue']];
    'change': [Props['modelValue']];
    'keyup': [KeyboardEvent];
}>();

const inputId
    = props.name ?? `input-${Math.random().toString(36).substring(2, 9)}`;

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
    const target = event.target as HTMLInputElement;

    inputValue.value = target.value;
    const resolvedValue = resolveValue(inputValue.value);

    emit('update:modelValue', resolvedValue);
};

const handleChange = (): void => {
    const resolvedValue = resolveValue(inputValue.value);

    emit('update:modelValue', resolvedValue);
    emit('change', resolvedValue);
};

watch(
    () => props.modelValue,
    (newValue) => {
        if (newValue === null || newValue === undefined) {
            inputValue.value = '';
        } else {
            inputValue.value = String(newValue);
        }
    },
    { immediate: true },
);

onMounted(() => {
    emit('update:modelValue', resolveValue(props.modelValue));
});
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
        <input
            :id="inputId"
            :name="name"
            :value="inputValue"
            :type="type || 'text'"
            :placeholder="placeholder"
            :disabled="disabled"
            class="w-full border rounded-lg shadow-sm bg-white
        transition-colors focus:outline-none focus:ring-2
        focus:ring-orange-500 focus:border-orange-500"
            :class="[
                size === 'xl'
                    ? 'px-6 py-4 text-lg'
                    : size === 'lg'
                        ? 'px-4 py-3 text-base'
                        : size === 'sm'
                            ? 'px-2 py-1 text-sm'
                            : 'px-3 py-2',
                withSpinner && (size === 'xl' || size === 'lg')
                    ? 'pr-12'
                    : '',
                error
                    ? 'border-red-300 focus:ring-red-500'
                    : 'border-gray-300 hover:border-gray-400',
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
