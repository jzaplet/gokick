<script setup lang="ts">
import type { DashboardResponse } from '@/app/Dashboard/types/DashboardResponse';
import { isDashboardResponse } from '@/app/Dashboard/types/DashboardResponse';
import { onMounted, ref } from 'vue';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import Spinner from '@/app-ui/Loading/Spinner.vue';

// The one dashboard fetch-and-show card (F-090): the admin and user dashboards
// differ only in the endpoint they read, so both views mount this component
// instead of carrying twin copies of the fetch/spinner/error logic.
const { endpoint } = defineProps<{
    endpoint: string;
}>();

const { error } = useToast();

const message = ref('');
const isLoading = ref(true);

onMounted(async (): Promise<void> => {
    const result = await authFetch<DashboardResponse>('GET', endpoint, { validate: isDashboardResponse });

    isLoading.value = false;

    if (result.success === false) {
        error('Failed to load dashboard.');

        return;
    }

    message.value = result.data.message;
});
</script>

<template>
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
</template>
