<script setup lang="ts">
import type { PlatformTenant } from '@/app/Platform/types/PlatformTenant';
import CheckBox from '@/app-ui/Inputs/CheckBox.vue';

// One tenant-overview row, rendered into DataGrid's #rows slot.
const { tenant, selectable, selected } = defineProps<{
    tenant: PlatformTenant;
    selectable?: boolean;
    selected?: boolean;
}>();

defineEmits<{
    toggleSelect: [tenant: PlatformTenant];
}>();
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
                :sr-label="`Select ${tenant.name}`"
                @update:model-value="$emit('toggleSelect', tenant)"
            />
        </td>
        <td class="px-4 py-3 whitespace-nowrap text-sm font-medium text-gray-900">
            {{ tenant.name }}
        </td>
        <td class="px-4 py-3 whitespace-nowrap text-sm">
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
        <td class="px-4 py-3 whitespace-nowrap text-right text-sm text-gray-500">
            {{ tenant.user_count }}
        </td>
    </tr>
</template>
