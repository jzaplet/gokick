<script setup lang="ts">
type Props = {
    class?: string;
};

defineProps<Props>();
</script>

<template>
    <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        :class="$props.class"
    >
        <path
            stroke-linecap="round"
            stroke-linejoin="round"
            d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
        />
    </svg>
</template>
