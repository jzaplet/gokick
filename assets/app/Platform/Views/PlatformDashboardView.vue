<script setup lang="ts">
import type { PlatformStats } from '@/app/Platform/types/PlatformStats';
import { isPlatformStats } from '@/app/Platform/types/PlatformStats';
import { onMounted, ref } from 'vue';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import Spinner from '@/app-ui/Loading/Spinner.vue';
import StatCard from '@/app-ui/Stats/StatCard.vue';

const { error } = useToast();

const stats = ref<PlatformStats | null>(null);
const isLoading = ref(true);

onMounted(async (): Promise<void> => {
    const result = await authFetch<PlatformStats>('GET', '/api/v1/platform/stats', {
        validate: isPlatformStats,
    });

    isLoading.value = false;

    if (result.success === false) {
        error('Failed to load platform stats.');

        return;
    }

    stats.value = result.data;
});
</script>

<template>
    <div class="space-y-6">
        <h1 class="text-2xl font-bold text-gray-900">
            Platform dashboard
        </h1>

        <div
            v-if="isLoading === true"
            class="flex items-center justify-center py-12"
        >
            <Spinner />
        </div>

        <div
            v-else-if="stats !== null"
            class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6"
        >
            <StatCard
                label="Tenants"
                :value="stats.tenant_count"
            />
            <StatCard
                label="Users"
                :value="stats.user_count"
            />
        </div>
    </div>
</template>
