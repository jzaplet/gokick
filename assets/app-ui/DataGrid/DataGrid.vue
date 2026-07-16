<script setup lang="ts">
import type { GridColumn, GridSort } from '@/app-ui/DataGrid/createGridState';
import ScrollShadow from '@/app-ui/ScrollShadow/ScrollShadow.vue';
import Spinner from '@/app-ui/Loading/Spinner.vue';

// The presentational half of the grid: header with sort affordances, loading
// row, card chrome and horizontal-scroll shadows. Rows are the CONSUMER's —
// the #rows slot renders domain <tr> components, so the grid never dictates
// cell markup (ported from aibobr).
const { columns, sort, isLoading } = defineProps<{
    columns: GridColumn[];
    sort: GridSort;
    isLoading: boolean;
}>();

const emit = defineEmits<{
    sort: [column: string];
}>();

const handleHeaderClick = (column: GridColumn): void => {
    if (column.sortable === true) {
        emit('sort', column.key);
    }
};
</script>

<template>
    <div class="bg-white rounded-lg shadow-md overflow-hidden">
        <ScrollShadow>
            <table class="min-w-full divide-y divide-gray-200">
                <thead class="bg-gray-50">
                    <tr>
                        <th
                            v-for="column in columns"
                            :key="column.key"
                            :class="[
                                'px-3 sm:px-6 py-3',
                                'text-xs font-medium text-gray-500 uppercase tracking-wider',
                                column.align === 'right' ? 'text-right' : 'text-left',
                                column.sortable === true ? 'cursor-pointer select-none hover:bg-gray-100' : '',
                            ]"
                            @click="handleHeaderClick(column)"
                        >
                            <span class="inline-flex items-center gap-1">
                                {{ column.label }}
                                <svg
                                    v-if="column.sortable === true && sort.column === column.key"
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    aria-hidden="true"
                                    class="w-3 h-3"
                                >
                                    <path
                                        stroke="currentColor"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                        stroke-width="2.5"
                                        :d="sort.direction === 'ASC' ? 'M12 19V5m-7 7 7-7 7 7' : 'M12 5v14m7-7-7 7-7-7'"
                                    />
                                </svg>
                            </span>
                        </th>
                    </tr>
                </thead>
                <tbody class="bg-white divide-y divide-gray-200">
                    <tr v-if="isLoading === true">
                        <td
                            :colspan="columns.length"
                            class="px-6 py-8 text-center"
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
    </div>
</template>
