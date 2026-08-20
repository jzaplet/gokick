<script setup lang="ts">
import type { AdminDashboardResponse } from '@/app/Dashboard/types/AdminDashboardResponse';
import { isAdminDashboardResponse } from '@/app/Dashboard/types/AdminDashboardResponse';
import { onMounted, ref } from 'vue';
import { authFetch } from '@/app-ui/Auth';
import { useI18n } from '@/app-ui/I18n';
import { useToast } from '@/app-ui/Toast/useToast';
import Spinner from '@/app-ui/Loading/Spinner.vue';
import StatCard from '@/app-ui/Stats/StatCard.vue';

const { error } = useToast();
const { t } = useI18n();

const stats = ref<AdminDashboardResponse | null>(null);
const isLoading = ref(true);

onMounted(async (): Promise<void> => {
    const result = await authFetch<AdminDashboardResponse>(
        'GET',
        '/api/v1/dashboard/admin',
        { validate: isAdminDashboardResponse },
    );

    isLoading.value = false;

    if (result.success === false) {
        error(t('dashboard.load_failed'));

        return;
    }

    stats.value = result.data;
});
</script>

<template>
    <div class="space-y-6">
        <h1 class="text-2xl font-bold text-gray-900">
            {{ t('dashboard.admin_title') }}
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
                :label="t('common.users')"
                :value="stats.users_active"
                :total="stats.users_total"
            />
        </div>
    </div>
</template>
