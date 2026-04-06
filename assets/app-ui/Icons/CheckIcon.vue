<script setup lang="ts">
type Props = {
    class?: string;
};

defineProps<Props>();
</script>

<template>
    <svg
        viewBox="0 0 12 12"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        :class="$props.class"
    >
        <path
            d="M2 6l3 3 5-6"
            stroke-linecap="round"
            stroke-linejoin="round"
        />
    </svg>
</template>
