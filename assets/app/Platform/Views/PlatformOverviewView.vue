<script setup lang="ts">
import type { PlatformTenant } from '@/app/Platform/types/PlatformTenant';
import type { PlatformUser } from '@/app/Platform/types/PlatformUser';
import { onMounted, ref } from 'vue';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import Spinner from '@/app-ui/Loading/Spinner.vue';
import PlatformTenantsTable from '@/app/Platform/Components/PlatformTenantsTable.vue';
import PlatformUsersTable from '@/app/Platform/Components/PlatformUsersTable.vue';

const { error } = useToast();

const tenants = ref<PlatformTenant[]>([]);
const users = ref<PlatformUser[]>([]);
const isLoading = ref(true);

const fetchOverview = async (): Promise<void> => {
    isLoading.value = true;

    const [tenantsResult, usersResult] = await Promise.all([
        authFetch<PlatformTenant[]>('GET', '/api/v1/platform/tenants'),
        authFetch<PlatformUser[]>('GET', '/api/v1/platform/users'),
    ]);

    isLoading.value = false;

    if (tenantsResult.success === false || usersResult.success === false) {
        error('Failed to load the platform overview.');

        return;
    }

    tenants.value = tenantsResult.data;
    users.value = usersResult.data;
};

onMounted(async (): Promise<void> => {
    await fetchOverview();
});
</script>

<template>
    <div class="py-12 px-4 sm:px-6 lg:px-8">
        <div class="max-w-5xl mx-auto space-y-8">
            <div>
                <h1 class="text-3xl font-extrabold text-gray-900">
                    Platform overview
                </h1>
                <p class="mt-1 text-sm text-gray-500">
                    Cross-tenant view for platform operators.
                </p>
            </div>

            <div
                v-if="isLoading === true"
                class="flex items-center justify-center py-12"
            >
                <Spinner />
            </div>

            <template v-else>
                <section class="space-y-3">
                    <h2 class="text-lg font-semibold text-gray-900">
                        Tenants
                    </h2>
                    <PlatformTenantsTable :tenants="tenants" />
                </section>

                <section class="space-y-3">
                    <h2 class="text-lg font-semibold text-gray-900">
                        Users
                    </h2>
                    <PlatformUsersTable :users="users" />
                </section>
            </template>
        </div>
    </div>
</template>
