<script setup lang="ts">
// Hover tooltip (ported from aibobr): pure CSS group-hover reveal, no JS
// positioning. Wraps its trigger in an inline-flex group; the bubble sits
// above (or below) with a rotated-square arrow. maxWidth switches the bubble
// from nowrap to a wrapped, centered block.
//
// align='right' anchors the bubble's RIGHT edge to the trigger instead of
// centring it. Needed by triggers that sit at the right edge of their container
// — a grid's actions column, for one: a centred bubble there hangs half its width
// past the viewport and gets clipped by the card's overflow-hidden. There is no
// JS measurement, so the caller picks; 'center' stays the default.
const { text, position = 'top', align = 'center', maxWidth = null } = defineProps<{
    text: string;
    position?: 'top' | 'bottom';
    align?: 'center' | 'right';
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
                'pointer-events-none absolute z-30',
                'rounded-md bg-gray-900 px-2.5 py-1.5 shadow-lg',
                'text-xs font-medium normal-case tracking-normal text-white',
                'opacity-0 transition-opacity duration-150 group-hover:opacity-100',
                align === 'right' ? 'right-0' : 'left-1/2 -translate-x-1/2',
                position === 'top' ? 'bottom-full mb-2' : 'top-full mt-2',
                maxWidth === null ? 'whitespace-nowrap' : 'whitespace-normal text-center',
            ]"
            :style="maxWidth === null ? undefined : { maxWidth: `${maxWidth}px`, width: 'max-content' }"
        >
            {{ text }}
            <!-- The arrow tracks the TRIGGER, not the bubble's centre: a
                 right-aligned bubble is offset from its trigger, so centring the
                 arrow on the bubble would point it at empty space. right-3 lands
                 it over the trigger, which is at the bubble's right edge. -->
            <span
                :class="[
                    'absolute h-2 w-2 rotate-45 bg-gray-900',
                    align === 'right' ? 'right-3' : 'left-1/2 -translate-x-1/2',
                    position === 'top' ? 'top-full -mt-1' : 'bottom-full -mb-1',
                ]"
            />
        </span>
    </span>
</template>
