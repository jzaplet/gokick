<script setup lang="ts">
import AppHeader from '@/app/Layout/AppHeader.vue';
</script>

<template>
    <div class="min-h-screen bg-gray-50 flex flex-col">
        <AppHeader />
        <main class="flex-1">
            <slot />
        </main>
    </div>
</template>
