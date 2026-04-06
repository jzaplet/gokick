<script setup lang="ts">
import CheckIcon from '@/app-ui/Icons/CheckIcon.vue';

type Props = {
    modelValue?: boolean;
    label?: string;
    error?: string;
    disabled?: boolean;
    name?: string;
};

const props = defineProps<Props>();
const emit = defineEmits<{
    'update:modelValue': [Props['modelValue']];
}>();

const inputId
    = props.name ?? `checkbox-${Math.random().toString(36).substring(2, 9)}`;

const toggle = (): void => {
    if (props.disabled) {
        return;
    }
    emit('update:modelValue', !props.modelValue);
};
</script>

<template>
    <div>
        <div class="flex items-center">
            <button
                :id="inputId"
                type="button"
                role="checkbox"
                :aria-checked="Boolean(modelValue)"
                :disabled="disabled"
                class="flex items-center justify-center h-4 w-4 rounded border
          transition-colors cursor-pointer
          focus:outline-none focus:ring-2 focus:ring-offset-2"
                :class="[
                    error
                        ? 'border-red-500 focus:ring-red-500'
                        : 'border-gray-300 focus:ring-orange-500',
                    modelValue && !error && 'bg-orange-500 border-orange-500',
                    modelValue && error && 'bg-red-500 border-red-500',
                    !modelValue && 'bg-white',
                    disabled && 'opacity-50 cursor-not-allowed',
                ]"
                @click="toggle"
            >
                <CheckIcon
                    v-if="modelValue"
                    class="h-3 w-3 text-white"
                />
            </button>
            <span
                v-if="label ?? $slots['default']"
                class="ml-2 text-sm text-gray-700 cursor-pointer select-none"
                @click="toggle"
            >
                <slot>{{ label }}</slot>
            </span>
        </div>
        <p
            v-if="error"
            class="mt-1 text-sm text-red-600"
        >
            {{ error }}
        </p>
    </div>
</template>
