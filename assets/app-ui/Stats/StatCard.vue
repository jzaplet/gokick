<script setup lang="ts">
import { useI18n } from '@/app-ui/I18n';

// One dashboard stat (aibobr parity): small gray label, the headline number
// big, and an optional "Total: N" subline when the headline is a subset
// (e.g. active users of all users).
const { label, value, total = null } = defineProps<{
    label: string;
    value: number;
    total?: number | null;
}>();

const { t } = useI18n();
</script>

<template>
    <div class="bg-white rounded-lg shadow p-6">
        <p class="text-sm font-medium text-gray-500">
            {{ label }}
        </p>
        <p class="mt-2 text-3xl font-bold text-gray-900">
            {{ value }}
        </p>
        <p
            v-if="total !== null"
            class="mt-1 text-sm text-gray-500"
        >
            {{ t('stats.total', { total }) }}
        </p>
    </div>
</template>
