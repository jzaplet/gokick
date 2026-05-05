<script setup lang="ts">
import type { DashboardResponse } from '@/app/Dashboard/types/DashboardResponse';
import { onMounted, ref } from 'vue';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import Spinner from '@/app-ui/Loading/Spinner.vue';

const { error } = useToast();

const message = ref('');
const isLoading = ref(true);

onMounted(async (): Promise<void> => {
    const result = await authFetch<DashboardResponse>('GET', '/api/v1/dashboard/admin');

    isLoading.value = false;

    if (result.success === false) {
        error('Failed to load dashboard.');

        return;
    }

    message.value = result.data.message;
});
</script>

<template>
    <div class="py-12 px-4 sm:px-6 lg:px-8">
        <div class="max-w-3xl mx-auto space-y-6">
            <h1 class="text-3xl font-extrabold text-gray-900">
                Admin dashboard
            </h1>

            <div class="bg-white rounded-lg shadow-md p-6">
                <div
                    v-if="isLoading === true"
                    class="flex items-center justify-center py-8"
                >
                    <Spinner />
                </div>
                <p
                    v-else
                    class="text-gray-700"
                >
                    {{ message }}
                </p>
            </div>
        </div>
    </div>
</template>
