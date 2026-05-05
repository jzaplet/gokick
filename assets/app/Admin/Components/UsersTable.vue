<script setup lang="ts">
import type { AdminUser } from '@/app/Admin/types/AdminUser';
import { useAuth } from '@/app-ui/Auth';
import Button from '@/app-ui/Buttons/Button.vue';
import EditIcon from '@/app-ui/Icons/EditIcon.vue';
import TrashIcon from '@/app-ui/Icons/TrashIcon.vue';

defineProps<{
    users: AdminUser[];
}>();

defineEmits<{
    edit: [user: AdminUser];
    delete: [user: AdminUser];
}>();

const { user: currentUser } = useAuth();

const isSelf = (id: string): boolean => {
    return currentUser.value !== null && currentUser.value.id === id;
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
                        {{ user.nickname }}
                        <span
                            v-if="isSelf(user.id) === true"
                            class="ml-2 text-xs text-gray-400"
                        >(you)</span>
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
                                user.role === 'admin'
                                    ? 'bg-orange-100 text-orange-800'
                                    : 'bg-gray-100 text-gray-800',
                            ]"
                        >
                            {{ user.role }}
                        </span>
                    </td>
                    <td class="px-3 sm:px-6 py-4 whitespace-nowrap text-right text-sm">
                        <div class="flex items-center justify-end gap-1 sm:gap-2">
                            <Button
                                variant="ghost"
                                size="sm"
                                @click="$emit('edit', user)"
                            >
                                <EditIcon />
                            </Button>
                            <Button
                                variant="ghost"
                                size="sm"
                                :disabled="isSelf(user.id)"
                                @click="$emit('delete', user)"
                            >
                                <TrashIcon />
                            </Button>
                        </div>
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
