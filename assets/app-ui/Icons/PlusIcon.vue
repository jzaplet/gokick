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
            d="M12 4v16m8-8H4"
        />
    </svg>
</template>
