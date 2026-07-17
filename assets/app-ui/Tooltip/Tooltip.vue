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
//
// align moves the BUBBLE only. The arrow always points at the trigger's centre
// and is positioned against the wrapper to keep that true at any trigger width —
// see the template.
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
        </span>
        <!-- The arrow points at the TRIGGER, so it is a sibling of the slot rather
             than a child of the bubble: absolute inside the bubble it would resolve
             against the BUBBLE's box and have to compensate for align — which it
             used to do with a fixed right-3, silently assuming a ~24px-wide trigger
             (the one icon button that motivated align). Out here it resolves
             against the wrapper, which IS the trigger, so left-1/2 centres it at
             any trigger width and align stops being its business.

             The vertical offsets close the bubble's gap from this side: the bubble
             sits mb-2/mt-2 clear of the trigger, and mb-1/mt-1 puts the arrow's
             8px square astride that edge — the same 4px overlap it had as a child. -->
        <span
            v-if="text !== ''"
            :class="[
                'pointer-events-none absolute z-30 h-2 w-2 rotate-45 bg-gray-900',
                'opacity-0 transition-opacity duration-150 group-hover:opacity-100',
                'left-1/2 -translate-x-1/2',
                position === 'top' ? 'bottom-full mb-1' : 'top-full mt-1',
            ]"
        />
    </span>
</template>
