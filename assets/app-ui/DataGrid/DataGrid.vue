<script setup lang="ts">
import type { GridColumn, GridSort } from '@/app-ui/DataGrid/createGridState';
import ScrollShadow from '@/app-ui/ScrollShadow/ScrollShadow.vue';
import Spinner from '@/app-ui/Loading/Spinner.vue';
import CheckBox from '@/app-ui/Inputs/CheckBox.vue';

// The presentational half of the grid: header with sort affordances, loading
// row and horizontal-scroll shadows. Rows are the CONSUMER's — the #rows slot
// renders domain <tr> components, so the grid never dictates cell markup. The
// card chrome (border, rounded corners) belongs to the view wrapping grid and
// pagination together (aibobr parity).
const { columns, sort, isLoading, selectable, allSelected } = defineProps<{
    columns: GridColumn[];
    sort: GridSort;
    isLoading: boolean;
    // Selection UI: the header checkbox toggles the current page (the grid
    // state owns WHICH ids that means); row checkboxes live in the consumer's
    // row components, same as all cell markup.
    selectable?: boolean;
    allSelected?: boolean;
}>();

const emit = defineEmits<{
    sort: [column: string];
    togglePage: [];
}>();

const handleHeaderClick = (column: GridColumn): void => {
    if (column.sortable === true) {
        emit('sort', column.key);
    }
};
</script>

<template>
    <ScrollShadow>
        <table class="w-full divide-y divide-gray-200">
            <thead class="bg-gray-50 border-b border-gray-300">
                <tr>
                    <th
                        v-if="selectable === true"
                        class="w-10 px-4 py-3"
                    >
                        <CheckBox
                            :model-value="allSelected"
                            sr-label="Select page"
                            @update:model-value="emit('togglePage')"
                        />
                    </th>
                    <th
                        v-for="column in columns"
                        :key="column.key"
                        :class="[
                            'px-4 py-3',
                            'text-xs font-medium text-gray-500 uppercase tracking-wider whitespace-nowrap',
                            column.align === 'right' ? 'text-right' : 'text-left',
                            column.sortable === true ? 'cursor-pointer hover:text-gray-700 select-none' : '',
                        ]"
                        @click="handleHeaderClick(column)"
                    >
                        <span class="inline-flex items-center gap-1">
                            {{ column.label }}
                            <template v-if="column.sortable === true">
                                <svg
                                    v-if="sort.column === column.key && sort.direction === 'ASC'"
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2.5"
                                    aria-hidden="true"
                                    class="w-3.5 h-3.5 text-gray-700"
                                >
                                    <path
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                        d="M5 15l7-7 7 7"
                                    />
                                </svg>
                                <svg
                                    v-else-if="sort.column === column.key"
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2.5"
                                    aria-hidden="true"
                                    class="w-3.5 h-3.5 text-gray-700"
                                >
                                    <path
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                        d="M19 9l-7 7-7-7"
                                    />
                                </svg>
                                <svg
                                    v-else
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2.5"
                                    aria-hidden="true"
                                    class="w-3.5 h-3.5 text-gray-300"
                                >
                                    <path
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                        d="M8 9l4-4 4 4"
                                    />
                                    <path
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                        d="M8 15l4 4 4-4"
                                    />
                                </svg>
                            </template>
                        </span>
                    </th>
                </tr>
            </thead>
            <tbody class="bg-white divide-y divide-gray-200">
                <tr v-if="isLoading === true">
                    <td
                        :colspan="selectable === true ? columns.length + 1 : columns.length"
                        class="px-4 py-8 text-center"
                    >
                        <Spinner />
                    </td>
                </tr>
                <slot
                    v-else
                    name="rows"
                />
            </tbody>
        </table>
    </ScrollShadow>
</template>
