<script setup lang="ts">
import { useId } from 'vue';

type Props = {
    class?: string;
};

defineProps<Props>();

// The counterchanged red saltire is the white one clipped to four quadrant
// wedges, and a clip path can only be referenced by id — a DOCUMENT-global
// name. useId() keeps a second instance of this icon on the same page from
// redefining the clip out from under the first.
const clipId = useId();
</script>

<template>
    <!--
        Filled multi-colour shapes, same divergence from the monochrome icon
        idiom as FlagCsIcon. Squeezed from the flag's true 2:1 into the 3:2 the
        switcher lines everything up at, so the saltire runs slightly steeper
        than on the real Union Flag — the usual trade every fixed-grid flag set
        makes, and the alternative (per-flag aspect ratios) is a ragged row.
    -->
    <svg
        viewBox="0 0 45 30"
        xmlns="http://www.w3.org/2000/svg"
        aria-hidden="true"
        :class="$props.class"
    >
        <clipPath :id="clipId">
            <path d="M22.5,15 h22.5 v15 z v15 h-22.5 z h-22.5 v-15 z v-15 h22.5 z" />
        </clipPath>
        <path
            fill="#00247d"
            d="M0 0h45v30H0z"
        />
        <path
            fill="none"
            stroke="#ffffff"
            stroke-width="6"
            d="M0,0 L45,30 M45,0 L0,30"
        />
        <path
            fill="none"
            stroke="#cf142b"
            stroke-width="4"
            :clip-path="`url(#${clipId})`"
            d="M0,0 L45,30 M45,0 L0,30"
        />
        <path
            fill="none"
            stroke="#ffffff"
            stroke-width="10"
            d="M22.5,0 v30 M0,15 h45"
        />
        <path
            fill="none"
            stroke="#cf142b"
            stroke-width="6"
            d="M22.5,0 v30 M0,15 h45"
        />
    </svg>
</template>
