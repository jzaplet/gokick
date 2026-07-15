<script setup lang="ts">
// The role-aware nav link row, rendered identically in both the desktop and the
// mobile <nav> in AppHeader. Extracted so the RouterLink loop (class list +
// active-class) has a single definition.
const { links } = defineProps<{ links: { name: string; label: string }[] }>();
</script>

<template>
    <RouterLink
        v-for="link in links"
        :key="link.name"
        :to="{ name: link.name }"
        :class="[
            'px-3 py-1.5 rounded-md text-sm font-medium',
            'text-gray-700 hover:text-gray-900 hover:bg-gray-100',
        ]"
        active-class="!text-orange-700 !bg-orange-50"
    >
        {{ link.label }}
    </RouterLink>
</template>
