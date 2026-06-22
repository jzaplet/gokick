<script setup lang="ts">
import type { PlatformUser } from '@/app/Platform/types/PlatformUser';

defineProps<{
    users: PlatformUser[];
}>();

const roleBadge = (role: string): string => {
    if (role === 'superadmin') {
        return 'bg-purple-100 text-purple-800';
    }
    if (role === 'admin') {
        return 'bg-orange-100 text-orange-800';
    }

    return 'bg-gray-100 text-gray-800';
};

const formatLastLogin = (value: string | null): string => {
    if (value === null) {
        return 'Never';
    }

    return new Date(value).toLocaleString();
};
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
                        Nickname
                    </th>
                    <th
                        :class="[
                            'px-3 sm:px-6 py-3',
                            'text-left text-xs font-medium text-gray-500 uppercase tracking-wider',
                        ]"
                    >
                        Role
                    </th>
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
                        Last login
                    </th>
                </tr>
            </thead>
            <tbody class="bg-white divide-y divide-gray-200">
                <tr
                    v-for="user in users"
                    :key="user.id"
                    class="hover:bg-gray-50"
                >
                    <td class="px-3 sm:px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">
                        {{ user.nickname }}
                    </td>
                    <td class="px-3 sm:px-6 py-4 whitespace-nowrap text-sm">
                        <span
                            :class="[
                                'inline-flex px-2 py-1',
                                'text-xs font-semibold rounded-full',
                                roleBadge(user.role),
                            ]"
                        >
                            {{ user.role }}
                        </span>
                    </td>
                    <td class="px-3 sm:px-6 py-4 whitespace-nowrap text-sm text-gray-500 font-mono">
                        {{ user.tenant_id }}
                    </td>
                    <td class="px-3 sm:px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                        {{ formatLastLogin(user.last_login_at) }}
                    </td>
                </tr>
                <tr v-if="users.length === 0">
                    <td
                        colspan="4"
                        class="px-6 py-8 text-center text-sm text-gray-500"
                    >
                        No users
                    </td>
                </tr>
            </tbody>
        </table>
    </div>
</template>
