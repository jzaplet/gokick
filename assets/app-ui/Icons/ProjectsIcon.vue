<script setup lang="ts">
type Props = {
    className?: string;
};

defineProps<Props>();
</script>

<template>
    <svg
        :class="className || 'mx-auto h-12 w-12 text-gray-400'"
        stroke="currentColor"
        fill="none"
        viewBox="0 0 48 48"
    >
        <path
            d="
        M34 40h10v-4a6 6 0 00-10.712-3.714M34 40H14m20 0v-4a9.971 9.971 0 00-.712-3.714
        M14 40H4v-4a6 6 0 0110.713-3.714M14 40v-4c0-1.313.253-2.566.713-3.714
        m0 0A9.971 9.971 0 0124 24c5.520 0 10 4.480 10 10
      "
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
        />
    </svg>
</template>
