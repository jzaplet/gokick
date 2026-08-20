<script setup lang="ts">
import DashboardCard from '@/app/Dashboard/Components/DashboardCard.vue';
import { useI18n } from '@/app-ui/I18n';

const { t } = useI18n();
</script>

<template>
    <div>
        <div class="max-w-3xl mx-auto space-y-6">
            <h1 class="text-2xl font-bold text-gray-900">
                {{ t('dashboard.user_title') }}
            </h1>

            <DashboardCard endpoint="/api/v1/dashboard/user" />
        </div>
    </div>
</template>
