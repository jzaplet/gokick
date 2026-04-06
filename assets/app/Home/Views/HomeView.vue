<script setup lang="ts">
import { onMounted } from 'vue';
import { apiFetch } from '@/app-ui/Fetch/useFetch';
import { useToast } from '@/app-ui/Toast/useToast';

const { success, error } = useToast();

type HealthResponse = {
    status: string;
};

onMounted(async (): Promise<void> => {
    const result = await apiFetch<HealthResponse>('GET', '/health');

    if (result.success === true) {
        success(`API Test status: ${result.data.status}`);
    } else {
        error(`API Test Error ${String(result.status)}`);
    }
});
</script>

<template>
    <div class="flex flex-col items-center justify-center min-h-screen p-10">
        <h1 class="text-4xl font-bold text-gray-900 text-center">
            This is your brand new app!
        </h1>
        <img
            src="@/img/go-vue-cqrs-ddd.png"
            alt="Go Vue CQRS DDD Logo"
            class="mt-8 max-w-full max-h-[50vh] object-contain"
        >
    </div>
</template>
