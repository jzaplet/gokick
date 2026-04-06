<script setup lang="ts">
type Props = {
    variant?: 'primary' | 'secondary' | 'danger' | 'ghost';
    size?: 'xs' | 'sm' | 'md' | 'lg';
    disabled?: boolean;
    loading?: boolean;
    type?: 'button' | 'submit' | 'reset';
};

const {
    variant = 'primary',
    size = 'md',
    type = 'button',
    disabled,
    loading,
} = defineProps<Props>();

defineEmits<{
    click: [MouseEvent];
}>();
</script>

<template>
    <button
        :type="type"
        :disabled="disabled || loading"
        class="inline-flex items-center justify-center gap-2 font-medium
      rounded-lg transition-colors cursor-pointer
      focus:outline-none focus:ring-2 focus:ring-offset-2"
        :class="[
            {
                'px-1.5 py-1 text-xs': size === 'xs',
                'px-3 py-1.5 text-sm': size === 'sm',
                'px-4 py-2 text-sm': size === 'md',
                'px-6 py-3 text-base': size === 'lg',
            },
            {
                'bg-orange-500 text-white hover:bg-orange-600 focus:ring-orange-500':
                    variant === 'primary' && !disabled,
                'bg-gray-200 text-gray-900 hover:bg-gray-300 focus:ring-gray-500':
                    variant === 'secondary' && !disabled,
                'bg-red-600 text-white hover:bg-red-700 focus:ring-red-500':
                    variant === 'danger' && !disabled,
                'bg-transparent text-gray-600 hover:bg-gray-100 focus:ring-gray-500':
                    variant === 'ghost' && !disabled,
                'opacity-50 cursor-not-allowed border border-gray-300 bg-gray-100 text-gray-500':
                    disabled || loading,
            },
        ]"
        @click="$emit('click', $event)"
    >
        <svg
            v-if="loading"
            class="animate-spin -ml-1 mr-2 h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
        >
            <circle
                class="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                stroke-width="4"
            />
            <path
                class="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2
          5.291A7.962 7.962 0 014 12H0c0 3.042 1.135
          5.824 3 7.938l3-2.647z"
            />
        </svg>
        <slot />
    </button>
</template>
