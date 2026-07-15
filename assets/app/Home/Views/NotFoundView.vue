<script setup lang="ts">
// Terminal catch-all view for the router's `/:pathMatch(.*)*` route, so an
// unmatched client path renders a real 404 page instead of a blank RouterView.
</script>

<template>
    <main
        :class="[
            'flex flex-col items-center justify-center',
            'min-h-[60vh] gap-4 text-center px-4',
        ]"
    >
        <p class="text-5xl font-bold text-gray-900">
            404
        </p>
        <p class="text-gray-600">
            This page could not be found.
        </p>
        <RouterLink
            :to="{ name: 'home' }"
            :class="[
                'px-4 py-2 rounded-lg',
                'text-sm font-medium text-white',
                'bg-orange-600 hover:bg-orange-700 transition-colors',
            ]"
        >
            Back to home
        </RouterLink>
    </main>
</template>
