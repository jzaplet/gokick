<script setup lang="ts">
import type { PlatformStats } from '@/app/Platform/types/PlatformStats';
import { onMounted, ref } from 'vue';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import Spinner from '@/app-ui/Loading/Spinner.vue';

const { error } = useToast();

const stats = ref<PlatformStats | null>(null);
const isLoading = ref(true);

onMounted(async (): Promise<void> => {
    const result = await authFetch<PlatformStats>('GET', '/api/v1/platform/stats');

    isLoading.value = false;

    if (result.success === false) {
        error('Failed to load platform stats.');

        return;
    }

    stats.value = result.data;
});
</script>

<template>
    <div class="py-12 px-4 sm:px-6 lg:px-8">
        <div class="max-w-4xl mx-auto space-y-6">
            <h1 class="text-3xl font-extrabold text-gray-900">
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
                class="grid grid-cols-1 sm:grid-cols-2 gap-6"
            >
                <div class="bg-white rounded-lg shadow-md p-6">
                    <p class="text-sm font-medium text-gray-500 uppercase tracking-wider">
                        Tenants
                    </p>
                    <p class="mt-2 text-4xl font-extrabold text-gray-900">
                        {{ stats.tenant_count }}
                    </p>
                </div>
                <div class="bg-white rounded-lg shadow-md p-6">
                    <p class="text-sm font-medium text-gray-500 uppercase tracking-wider">
                        Users
                    </p>
                    <p class="mt-2 text-4xl font-extrabold text-gray-900">
                        {{ stats.user_count }}
                    </p>
                </div>
            </div>
        </div>
    </div>
</template>
