<script setup lang="ts">
import type { PlatformTenant } from '@/app/Platform/types/PlatformTenant';

defineProps<{
    tenants: PlatformTenant[];
}>();
</script>

<template>
    <div class="bg-white rounded-lg shadow-md overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-200">
            <thead class="bg-gray-50">
                <tr>
                    <th
                        :class="[
                            'px-3 sm:px-6 py-3',
                            'text-left text-xs font-medium text-gray-500 uppercase tracking-wider',
                        ]"
                    >
                        Tenant
                    </th>
                    <th
                        :class="[
                            'px-3 sm:px-6 py-3',
                            'text-left text-xs font-medium text-gray-500 uppercase tracking-wider',
                        ]"
                    >
                        Plan
                    </th>
                    <th
                        :class="[
                            'px-3 sm:px-6 py-3 text-right',
                            'text-xs font-medium text-gray-500 uppercase tracking-wider',
                        ]"
                    >
                        Users
                    </th>
                </tr>
            </thead>
            <tbody class="bg-white divide-y divide-gray-200">
                <tr
                    v-for="tenant in tenants"
                    :key="tenant.id"
                    class="hover:bg-gray-50"
                >
                    <td class="px-3 sm:px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">
                        {{ tenant.name }}
                    </td>
                    <td class="px-3 sm:px-6 py-4 whitespace-nowrap text-sm">
                        <span
                            :class="[
                                'inline-flex px-2 py-1',
                                'text-xs font-semibold rounded-full',
                                tenant.plan === 'free'
                                    ? 'bg-gray-100 text-gray-800'
                                    : 'bg-green-100 text-green-800',
                            ]"
                        >
                            {{ tenant.plan }}
                        </span>
                    </td>
                    <td class="px-3 sm:px-6 py-4 whitespace-nowrap text-right text-sm text-gray-500">
                        {{ tenant.user_count }}
                    </td>
                </tr>
                <tr v-if="tenants.length === 0">
                    <td
                        colspan="3"
                        class="px-6 py-8 text-center text-sm text-gray-500"
                    >
                        No tenants
                    </td>
                </tr>
            </tbody>
        </table>
    </div>
</template>
