<script setup lang="ts">
import type { PlatformUser } from '@/app/Platform/types/PlatformUser';
import { Role } from '@/app/Auth/enums/roles';
import { roleBadge } from '@/app-ui/Users/roleBadge';
import Button from '@/app-ui/Buttons/Button.vue';
import CheckBox from '@/app-ui/Inputs/CheckBox.vue';
import EditIcon from '@/app-ui/Icons/EditIcon.vue';
import TapIcon from '@/app-ui/Icons/TapIcon.vue';
import TrashIcon from '@/app-ui/Icons/TrashIcon.vue';
import Tooltip from '@/app-ui/Tooltip/Tooltip.vue';

// One platform-list row (cross-tenant), rendered into DataGrid's #rows slot.
const { user, selectable, selected } = defineProps<{
    user: PlatformUser;
    selectable?: boolean;
    selected?: boolean;
}>();

defineEmits<{
    edit: [user: PlatformUser];
    delete: [user: PlatformUser];
    activate: [user: PlatformUser];
    toggleSelect: [user: PlatformUser];
}>();

const formatLastLogin = (value: string | null): string => {
    if (value === null) {
        return 'Never';
    }

    return new Date(value).toLocaleString();
};

// A superadmin row is managed out-of-band — the backend rejects edit/delete on
// it, so the actions are disabled here to match.
const isManageable = (): boolean => {
    return user.role !== Role.SuperAdmin;
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
                :disabled="isManageable() === false"
                :sr-label="`Select ${user.nickname}`"
                @update:model-value="$emit('toggleSelect', user)"
            />
        </td>
        <td class="px-4 py-3 whitespace-nowrap text-sm font-medium text-gray-900">
            {{ user.nickname }}
        </td>
        <td class="px-4 py-3 whitespace-nowrap text-sm text-gray-500">
            {{ user.tenant_name }}
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
        <td class="px-4 py-3 whitespace-nowrap text-sm text-gray-500">
            {{ formatLastLogin(user.last_login_at) }}
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
                        aria-label="Activate user"
                        :disabled="isManageable() === false"
                        @click="$emit('activate', user)"
                    >
                        <TapIcon />
                    </Button>
                </Tooltip>
                <Button
                    variant="secondary"
                    size="xs"
                    :disabled="isManageable() === false"
                    @click="$emit('edit', user)"
                >
                    <EditIcon />
                </Button>
                <Button
                    variant="danger"
                    size="xs"
                    :disabled="isManageable() === false"
                    @click="$emit('delete', user)"
                >
                    <TrashIcon />
                </Button>
            </div>
        </td>
    </tr>
</template>
