<script setup lang="ts">
type Props = {
    class?: string;
};

defineProps<Props>();
</script>

<template>
    <!--
        Flags are filled multi-colour shapes, so this deliberately breaks the
        monochrome `fill="none" stroke="currentColor"` idiom of the other icons
        here: explicit fills, no currentColor. Callers desaturate it with a CSS
        filter (the LangSwitcher's grayscale state) rather than by recolouring.
        Drawn 3:2 — exact for this flag, and the shared ratio that lets the
        switcher line several flags up at one size.
    -->
    <svg
        viewBox="0 0 45 30"
        xmlns="http://www.w3.org/2000/svg"
        aria-hidden="true"
        :class="$props.class"
    >
        <path
            fill="#ffffff"
            d="M0 0h45v15H0z"
        />
        <path
            fill="#d7141a"
            d="M0 15h45v15H0z"
        />
        <path
            fill="#11457e"
            d="M0 0l22.5 15L0 30z"
        />
    </svg>
</template>
