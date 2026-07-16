<script setup lang="ts">
import AppHeader from '@/app/Layout/AppHeader.vue';
import AppSidebar from '@/app/Layout/AppSidebar.vue';
import { useSidebar } from '@/app/Layout/useSidebar';

const { isSidebarOpen } = useSidebar();
</script>

<template>
    <div class="min-h-screen bg-gray-50">
        <AppSidebar />
        <div
            :class="[
                'flex flex-col min-h-screen min-w-0',
                'transition-all duration-200',
                isSidebarOpen === true ? 'md:ml-64' : 'ml-0',
            ]"
        >
            <AppHeader />
            <main class="flex-1 min-w-0">
                <slot />
            </main>
        </div>
    </div>
</template>
