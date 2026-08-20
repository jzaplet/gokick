<script setup lang="ts">
import type { PlatformTenant } from '@/app/Platform/types/PlatformTenant';
import { useI18n } from '@/app-ui/I18n';
import type { TranslationKey } from '@/app-ui/I18n';
import CheckBox from '@/app-ui/Inputs/CheckBox.vue';
import Button from '@/app-ui/Buttons/Button.vue';
import Tooltip from '@/app-ui/Tooltip/Tooltip.vue';
import TrashIcon from '@/app-ui/Icons/TrashIcon.vue';

// One tenant-overview row, rendered into DataGrid's #rows slot.
const { tenant, selectable, selected } = defineProps<{
    tenant: PlatformTenant;
    selectable?: boolean;
    selected?: boolean;
}>();

defineEmits<{
    toggleSelect: [tenant: PlatformTenant];
    delete: [tenant: PlatformTenant];
}>();

const { t } = useI18n();

// Why this row cannot be deleted, or '' when it can. Doubles as the tooltip text
// and the disabled test, so the button and its explanation can never disagree.
//
// This is a HINT, not the gate: user_count was already stale when the grid
// rendered, and the backend re-tests emptiness inside the DELETE itself. It
// exists to explain, not to protect.
const blockedReason = (): string => {
    if (tenant.is_default === true) {
        return t('tenants.default_undeletable');
    }
    if (tenant.user_count > 0) {
        return t('tenants.has_users', { count: tenant.user_count });
    }

    return '';
};

// Display names for the wire plan values; an unknown plan (a tier newer than
// this build) falls back to the raw value rather than rendering nothing.
const planKeys: Record<string, TranslationKey> = {
    free: 'plan.free',
};

const planLabel = (plan: string): string => {
    const key = planKeys[plan];

    return key === undefined ? plan : t(key);
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
                :sr-label="t('tenants.select', { name: tenant.name })"
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
                {{ planLabel(tenant.plan) }}
            </span>
        </td>
        <td class="px-4 py-3 whitespace-nowrap text-right text-sm text-gray-500">
            {{ tenant.user_count }}
        </td>
        <td class="px-4 py-3 whitespace-nowrap text-right text-sm">
            <div class="flex items-center justify-end gap-1">
                <!-- Right-aligned: this is the last column, so a centred bubble
                     would hang past the viewport and be clipped by the card. -->
                <Tooltip
                    :text="blockedReason()"
                    align="right"
                    :max-width="220"
                >
                    <Button
                        variant="danger"
                        size="xs"
                        :aria-label="t('tenants.delete_title')"
                        :disabled="blockedReason() !== ''"
                        @click="$emit('delete', tenant)"
                    >
                        <TrashIcon />
                    </Button>
                </Tooltip>
            </div>
        </td>
    </tr>
</template>
