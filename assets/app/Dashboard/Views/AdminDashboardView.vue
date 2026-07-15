<script setup lang="ts">
import DashboardCard from '@/app/Dashboard/Components/DashboardCard.vue';
</script>

<template>
    <div class="py-12 px-4 sm:px-6 lg:px-8">
        <div class="max-w-3xl mx-auto space-y-6">
            <h1 class="text-3xl font-extrabold text-gray-900">
                Admin dashboard
            </h1>

            <DashboardCard endpoint="/api/v1/dashboard/admin" />
        </div>
    </div>
</template>
