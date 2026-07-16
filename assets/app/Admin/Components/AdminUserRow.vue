<script setup lang="ts">
import type { AdminUser } from '@/app/Admin/types/AdminUser';
import { useAuth } from '@/app-ui/Auth';
import { roleBadge } from '@/app-ui/Users/roleBadge';
import Button from '@/app-ui/Buttons/Button.vue';
import CheckBox from '@/app-ui/Inputs/CheckBox.vue';
import EditIcon from '@/app-ui/Icons/EditIcon.vue';
import TapIcon from '@/app-ui/Icons/TapIcon.vue';
import TrashIcon from '@/app-ui/Icons/TrashIcon.vue';
import Tooltip from '@/app-ui/Tooltip/Tooltip.vue';

// One admin-list row, rendered into DataGrid's #rows slot (the grid never
// dictates cell markup). Self-delete stays disabled — an admin must not saw
// off the branch they sit on.
const { user, selectable, selected } = defineProps<{
    user: AdminUser;
    selectable?: boolean;
    selected?: boolean;
}>();

defineEmits<{
    edit: [user: AdminUser];
    delete: [user: AdminUser];
    activate: [user: AdminUser];
    toggleSelect: [user: AdminUser];
}>();

const { user: currentUser } = useAuth();

const isSelf = (): boolean => {
    return currentUser.value !== null && currentUser.value.id === user.id;
};
</script>

<template>
    <tr
        :class="[
            'hover:bg-gray-50 transition-colors',
            selected === true ? 'bg-orange-50' : '',
        ]"
    >
        <td
            v-if="selectable === true"
            class="w-10 px-4 py-3"
        >
            <CheckBox
                :model-value="selected"
                :disabled="isSelf()"
                :sr-label="`Select ${user.nickname}`"
                @update:model-value="$emit('toggleSelect', user)"
            />
        </td>
        <td class="px-4 py-3 whitespace-nowrap text-sm font-medium text-gray-900">
            {{ user.nickname }}
            <span
                v-if="isSelf() === true"
                class="ml-2 text-xs text-gray-400"
            >(you)</span>
        </td>
        <td class="px-4 py-3 whitespace-nowrap text-sm text-gray-500">
            <span v-if="user.email !== ''">{{ user.email }}</span>
            <span
                v-else
                class="text-gray-300"
            >—</span>
        </td>
        <td class="px-4 py-3 whitespace-nowrap text-sm">
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
        <td class="px-4 py-3 whitespace-nowrap text-sm">
            <span
                :class="[
                    'inline-flex px-2 py-1',
                    'text-xs font-semibold rounded-full',
                    user.active === true
                        ? 'bg-green-100 text-green-800'
                        : 'bg-gray-100 text-gray-600',
                ]"
            >
                {{ user.active === true ? 'Active' : 'Inactive' }}
            </span>
        </td>
        <td class="px-4 py-3 whitespace-nowrap text-right text-sm">
            <div class="flex items-center justify-end gap-1">
                <Tooltip
                    v-if="user.active === false"
                    text="Activate user"
                >
                    <Button
                        variant="secondary"
                        size="xs"
                        @click="$emit('activate', user)"
                    >
                        <TapIcon />
                    </Button>
                </Tooltip>
                <Button
                    variant="secondary"
                    size="xs"
                    @click="$emit('edit', user)"
                >
                    <EditIcon />
                </Button>
                <Button
                    variant="danger"
                    size="xs"
                    :disabled="isSelf()"
                    @click="$emit('delete', user)"
                >
                    <TrashIcon />
                </Button>
            </div>
        </td>
    </tr>
</template>
