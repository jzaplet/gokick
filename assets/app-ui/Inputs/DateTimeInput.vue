<script setup lang="ts">
import { onMounted, ref, watch } from 'vue';

type Props = {
    modelValue?: null | string;
    defaultValue?: string;
    placeholder?: string;
    label?: string;
    error?: string;
    required?: boolean;
    disabled?: boolean;
    name?: string;
    isNullable?: boolean;
};

const props = defineProps<Props>();
const emit = defineEmits<{
    'update:modelValue': [Props['modelValue']];
    'change': [Props['modelValue']];
    'keyup': [KeyboardEvent];
}>();

const inputId
    = props.name
        ?? `datetime-input-${Math.random().toString(36).substring(2, 9)}`;

const inputValue = ref(props.defaultValue ?? '');

const resolveValue = (value: Props['modelValue']): string | null => {
    if (
        props.isNullable
        && (value === null || value === undefined || value === '')
    ) {
        return null;
    }

    return value ?? '';
};

const handleInput = (event: Event): void => {
    const target = event.target as HTMLInputElement;

    inputValue.value = target.value;
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
            inputValue.value = newValue;
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
            type="datetime-local"
            :placeholder="placeholder"
            :disabled="disabled"
            class="w-full px-3 py-2 border rounded-lg shadow-sm
        transition-colors focus:outline-none focus:ring-2
        focus:ring-orange-500 focus:border-orange-500"
            :class="[
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
    </div>
</template>
