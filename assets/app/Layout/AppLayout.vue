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
            <!-- One shared content shell (aibobr parity): every routed view
                 gets the same max width, padding and centering — grids and
                 dashboards line up across pages without per-view wrappers. -->
            <main
                :class="[
                    'flex-1 min-w-0',
                    'w-full max-w-[1800px] mx-auto',
                    'py-8 px-4 sm:px-6 lg:px-10 overflow-x-hidden',
                ]"
            >
                <slot />
            </main>
        </div>
    </div>
</template>
