<script setup lang="ts">
// Hover tooltip (ported from aibobr): pure CSS group-hover reveal, no JS
// positioning. Wraps its trigger in an inline-flex group; the bubble sits
// above (or below) with a rotated-square arrow. maxWidth switches the bubble
// from nowrap to a wrapped, centered block.
const { text, position = 'top', maxWidth = null } = defineProps<{
    text: string;
    position?: 'top' | 'bottom';
    maxWidth?: number | null;
}>();
</script>

<template>
    <span class="relative inline-flex group">
        <slot />
        <span
            v-if="text !== ''"
            role="tooltip"
            :class="[
                'pointer-events-none absolute left-1/2 z-30 -translate-x-1/2',
                'rounded-md bg-gray-900 px-2.5 py-1.5 shadow-lg',
                'text-xs font-medium normal-case tracking-normal text-white',
                'opacity-0 transition-opacity duration-150 group-hover:opacity-100',
                position === 'top' ? 'bottom-full mb-2' : 'top-full mt-2',
                maxWidth === null ? 'whitespace-nowrap' : 'whitespace-normal text-center',
            ]"
            :style="maxWidth === null ? undefined : { maxWidth: `${maxWidth}px`, width: 'max-content' }"
        >
            {{ text }}
            <span
                :class="[
                    'absolute left-1/2 h-2 w-2 -translate-x-1/2 rotate-45 bg-gray-900',
                    position === 'top' ? 'top-full -mt-1' : 'bottom-full -mb-1',
                ]"
            />
        </span>
    </span>
</template>
