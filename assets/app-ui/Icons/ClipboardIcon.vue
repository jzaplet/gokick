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
            d="
        M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2
        M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2
      "
        />
    </svg>
</template>
