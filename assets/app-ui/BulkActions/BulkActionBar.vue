<script setup lang="ts">
import Button from '@/app-ui/Buttons/Button.vue';

export type BulkAction = {
    key: string;
    label: string;
    variant?: 'primary' | 'secondary' | 'danger' | 'ghost';
};

// The action bar over a grid selection (ported from aibobr). Appears once
// anything is selected; offers the escalation to "all N filtered" — the grid
// then sends the FILTER SET to the backend instead of enumerating ids.
const { count, total, isAllFiltered, actions } = defineProps<{
    count: number;
    total: number;
    isAllFiltered: boolean;
    actions: BulkAction[];
}>();

const emit = defineEmits<{
    action: [key: string];
    selectAllFiltered: [];
    clear: [];
}>();
</script>

<template>
    <div
        v-if="count > 0"
        :class="[
            'flex flex-col sm:flex-row sm:items-center justify-between gap-3',
            'px-4 sm:px-6 py-3',
            'bg-orange-50 border border-orange-200 rounded-lg',
        ]"
    >
        <div class="flex items-center gap-3 text-sm text-gray-800">
            <span class="font-medium">
                {{ isAllFiltered === true ? `All ${count} filtered selected` : `${count} selected` }}
            </span>
            <button
                v-if="isAllFiltered === false && count < total"
                type="button"
                class="text-orange-700 hover:text-orange-900 underline cursor-pointer"
                @click="emit('selectAllFiltered')"
            >
                Select all {{ total }} filtered
            </button>
        </div>

        <div class="flex items-center gap-2">
            <Button
                v-for="action in actions"
                :key="action.key"
                :variant="action.variant ?? 'secondary'"
                size="sm"
                @click="emit('action', action.key)"
            >
                {{ action.label }}
            </Button>
            <Button
                variant="ghost"
                size="sm"
                @click="emit('clear')"
            >
                Clear
            </Button>
        </div>
    </div>
</template>
