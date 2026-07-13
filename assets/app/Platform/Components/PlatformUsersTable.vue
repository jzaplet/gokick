<script setup lang="ts">
import type { PlatformUser } from '@/app/Platform/types/PlatformUser';
import { roleBadge } from '@/app-ui/Users/roleBadge';
import Button from '@/app-ui/Buttons/Button.vue';
import EditIcon from '@/app-ui/Icons/EditIcon.vue';
import TrashIcon from '@/app-ui/Icons/TrashIcon.vue';

defineProps<{
    users: PlatformUser[];
}>();

defineEmits<{
    edit: [user: PlatformUser];
    delete: [user: PlatformUser];
}>();

const formatLastLogin = (value: string | null): string => {
    if (value === null) {
        return 'Never';
    }

    return new Date(value).toLocaleString();
};

// A superadmin row is managed out-of-band — the backend rejects edit/delete on
// it, so the actions are disabled here to match.
const isManageable = (role: string): boolean => {
    return role !== 'superadmin';
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
                        Tenant
                    </th>
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
                        Email
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
                        Last login
                    </th>
                    <th
                        :class="[
                            'px-3 sm:px-6 py-3 w-28 sm:w-32',
                            'text-right text-xs font-medium text-gray-500 uppercase tracking-wider',
                        ]"
                    >
                        Actions
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
                        {{ user.tenant_name }}
                    </td>
                    <td class="px-3 sm:px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                        {{ user.nickname }}
                    </td>
                    <td class="px-3 sm:px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                        <span v-if="user.email !== ''">{{ user.email }}</span>
                        <span
                            v-else
                            class="text-gray-300"
                        >—</span>
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
                    <td class="px-3 sm:px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                        {{ formatLastLogin(user.last_login_at) }}
                    </td>
                    <td class="px-3 sm:px-6 py-4 whitespace-nowrap text-right text-sm">
                        <div class="flex items-center justify-end gap-1 sm:gap-2">
                            <Button
                                variant="ghost"
                                size="sm"
                                :disabled="isManageable(user.role) === false"
                                @click="$emit('edit', user)"
                            >
                                <EditIcon />
                            </Button>
                            <Button
                                variant="ghost"
                                size="sm"
                                :disabled="isManageable(user.role) === false"
                                @click="$emit('delete', user)"
                            >
                                <TrashIcon />
                            </Button>
                        </div>
                    </td>
                </tr>
                <tr v-if="users.length === 0">
                    <td
                        colspan="6"
                        class="px-6 py-8 text-center text-sm text-gray-500"
                    >
                        No users
                    </td>
                </tr>
            </tbody>
        </table>
    </div>
</template>
