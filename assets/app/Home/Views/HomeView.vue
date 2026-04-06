<script setup lang="ts">
import { onMounted, ref } from 'vue';

type HealthResponse = {
    status: string;
};

const healthStatus = ref<string | null>(null);
const healthError = ref<string | null>(null);

onMounted(async (): Promise<void> => {
    try {
        const response = await fetch('/health');

        if (response.ok) {
            const data = await response.json() as HealthResponse;

            healthStatus.value = data.status;
        } else {
            healthError.value = `Error ${String(response.status)}`;
        }
    } catch {
        healthError.value = 'Connection failed';
    }
});
</script>

<template>
    <div class="flex flex-col items-center justify-center min-h-screen gap-4">
        <h1 class="text-4xl font-bold text-gray-900">
            MyApp
        </h1>
        <div
            v-if="healthStatus !== null"
            class="text-sm text-green-600"
        >
            API: {{ healthStatus }}
        </div>
        <div
            v-if="healthError !== null"
            class="text-sm text-red-600"
        >
            API: {{ healthError }}
        </div>
    </div>
</template>
