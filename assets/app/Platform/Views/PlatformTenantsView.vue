<script setup lang="ts">
import type { PlatformTenant } from '@/app/Platform/types/PlatformTenant';
import { isPlatformTenant } from '@/app/Platform/types/PlatformTenant';
import { arrayOf } from '@/app-ui/Fetch';
import { onMounted, ref } from 'vue';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import Spinner from '@/app-ui/Loading/Spinner.vue';
import PlatformTenantsTable from '@/app/Platform/Components/PlatformTenantsTable.vue';

const { error } = useToast();

const tenants = ref<PlatformTenant[]>([]);
const isLoading = ref(true);

onMounted(async (): Promise<void> => {
    const result = await authFetch<PlatformTenant[]>('GET', '/api/v1/platform/tenants', {
        validate: arrayOf(isPlatformTenant),
    });

    isLoading.value = false;

    if (result.success === false) {
        error('Failed to load tenants.');

        return;
    }

    tenants.value = result.data;
});
</script>

<template>
    <div class="py-12 px-4 sm:px-6 lg:px-8">
        <div class="max-w-5xl mx-auto space-y-6">
            <h1 class="text-3xl font-extrabold text-gray-900">
                Tenants
            </h1>

            <div
                v-if="isLoading === true"
                class="flex items-center justify-center py-12"
            >
                <Spinner />
            </div>

            <PlatformTenantsTable
                v-else
                :tenants="tenants"
            />
        </div>
    </div>
</template>
