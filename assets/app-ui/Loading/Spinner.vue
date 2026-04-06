<script setup lang="ts">
type Props = {
    size?: 'sm' | 'md' | 'lg';
    color?: 'primary' | 'secondary' | 'white' | 'success';
};

const { size = 'md', color = 'primary' } = defineProps<Props>();
</script>

<template>
    <div
        class="animate-spin rounded-full border-2 border-solid"
        :class="[
            {
                'h-4 w-4 border-t-2': size === 'sm',
                'h-6 w-6 border-t-2': size === 'md',
                'h-8 w-8 border-t-4': size === 'lg',
            },
            {
                'border-orange-500 border-t-transparent': color === 'primary',
                'border-gray-400 border-t-transparent': color === 'secondary',
                'border-white border-t-transparent': color === 'white',
                'border-green-600 border-t-transparent': color === 'success',
            },
        ]"
    />
</template>
