<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue';
import { edgeState } from '@/app-ui/ScrollShadow/edgeState';

// Horizontal-scroll wrapper with fade hints on the edges: when the content is
// wider than the viewport, a gradient appears on the side(s) with more content
// — an affordance plain `overflow-x-auto` lacks. Ported from the aibobr grid.
const viewport = ref<HTMLElement | null>(null);
const showLeft = ref(false);
const showRight = ref(false);

const update = (): void => {
    const el = viewport.value;

    if (el === null) {
        return;
    }

    const edges = edgeState(el.scrollLeft, el.scrollWidth, el.clientWidth);

    showLeft.value = edges.left;
    showRight.value = edges.right;
};

let observer: ResizeObserver | null = null;

onMounted(() => {
    update();

    // Sizes change without scroll events (viewport resize, rows loading in) —
    // observe both the viewport and its content. Guarded: jsdom has no
    // ResizeObserver, and without one the scroll listener still covers the
    // common case.
    if (typeof ResizeObserver !== 'undefined') {
        observer = new ResizeObserver(update);

        if (viewport.value !== null) {
            observer.observe(viewport.value);

            for (const child of viewport.value.children) {
                observer.observe(child);
            }
        }
    }
});

onBeforeUnmount(() => {
    observer?.disconnect();
});
</script>

<template>
    <div class="relative">
        <div
            v-if="showLeft === true"
            :class="[
                'pointer-events-none absolute inset-y-0 left-0 z-10 w-6',
                'bg-gradient-to-r from-gray-900/10 to-transparent',
            ]"
            aria-hidden="true"
        />
        <div
            ref="viewport"
            class="overflow-x-auto"
            @scroll.passive="update"
        >
            <slot />
        </div>
        <div
            v-if="showRight === true"
            :class="[
                'pointer-events-none absolute inset-y-0 right-0 z-10 w-6',
                'bg-gradient-to-l from-gray-900/10 to-transparent',
            ]"
            aria-hidden="true"
        />
    </div>
</template>
