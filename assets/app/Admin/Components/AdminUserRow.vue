<script setup lang="ts">
import type { AdminUser } from '@/app/Admin/types/AdminUser';
import { useAuth } from '@/app-ui/Auth';
import { roleBadge } from '@/app-ui/Users/roleBadge';
import Button from '@/app-ui/Buttons/Button.vue';
import EditIcon from '@/app-ui/Icons/EditIcon.vue';
import TrashIcon from '@/app-ui/Icons/TrashIcon.vue';

// One admin-list row, rendered into DataGrid's #rows slot (the grid never
// dictates cell markup). Self-delete stays disabled — an admin must not saw
// off the branch they sit on.
const { user } = defineProps<{
    user: AdminUser;
}>();

defineEmits<{
    edit: [user: AdminUser];
    delete: [user: AdminUser];
}>();

const { user: currentUser } = useAuth();

const isSelf = (): boolean => {
    return currentUser.value !== null && currentUser.value.id === user.id;
};
</script>

<template>
    <tr class="hover:bg-gray-50">
        <td class="px-3 sm:px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">
            {{ user.nickname }}
            <span
                v-if="isSelf() === true"
                class="ml-2 text-xs text-gray-400"
            >(you)</span>
            <span
                v-if="user.active === false"
                :class="[
                    'ml-2 inline-flex px-2 py-0.5',
                    'text-xs font-semibold rounded-full',
                    'bg-gray-100 text-gray-600',
                ]"
            >Inactive</span>
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
                    :disabled="isSelf()"
                    @click="$emit('delete', user)"
                >
                    <TrashIcon />
                </Button>
            </div>
        </td>
    </tr>
</template>
