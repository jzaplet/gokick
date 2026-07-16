<script setup lang="ts">
import type { PlatformTenant } from '@/app/Platform/types/PlatformTenant';
import type { PlatformTenantListResponse } from '@/app/Platform/types/PlatformTenantListResponse';
import { isPlatformTenantListResponse } from '@/app/Platform/types/PlatformTenantListResponse';
import type { GridColumn } from '@/app-ui/DataGrid/createGridState';
import { createGridState } from '@/app-ui/DataGrid/createGridState';
import { computed, onMounted, ref } from 'vue';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import DataGrid from '@/app-ui/DataGrid/DataGrid.vue';
import FilterPanel from '@/app-ui/FilterPanel/FilterPanel.vue';
import Pagination from '@/app-ui/Pagination/Pagination.vue';
import Input from '@/app-ui/Inputs/Input.vue';
import Select from '@/app-ui/Inputs/Select.vue';
import BulkActionBar from '@/app-ui/BulkActions/BulkActionBar.vue';
import PlatformTenantRow from '@/app/Platform/Components/PlatformTenantRow.vue';

const { error } = useToast();

const tenants = ref<PlatformTenant[]>([]);

const columns: GridColumn[] = [
    { key: 'name', label: 'Tenant', sortable: true },
    { key: 'plan', label: 'Plan' },
    { key: 'users', label: 'Users', sortable: true, align: 'right' },
];

const planOptions = [
    { value: '', label: 'All plans' },
    { value: 'free', label: 'free' },
];

const grid = createGridState({
    defaultSort: { column: 'name', direction: 'ASC' },
    filters: { name: '', plan: '' },
    syncUrl: true,
    load: async ({ page, perPage, sort, filters }) => {
        const params = new URLSearchParams({
            page: String(page),
            per_page: String(perPage),
            sort_by: sort.column,
            sort_dir: sort.direction,
        });

        for (const [key, value] of Object.entries(filters)) {
            if (value !== '') {
                params.set(key, value);
            }
        }

        const result = await authFetch<PlatformTenantListResponse>(
            'GET',
            `/api/v1/platform/tenants?${params.toString()}`,
            { validate: isPlatformTenantListResponse },
        );

        if (result.success === false) {
            error('Failed to load tenants.');

            return { ok: false };
        }

        tenants.value = result.data.items;

        return { ok: true, total: result.data.total };
    },
});

// No bulk operations exist for tenants YET — the bar still offers the
// selection mechanics (select page / all filtered / clear), so actions can
// land here later without UI work.
const pageIds = computed<string[]>(() => tenants.value.map((t) => t.id));

const allPageSelected = computed<boolean>(() =>
    pageIds.value.length > 0
    && pageIds.value.every((id) => grid.isSelected(id) === true));

onMounted(async (): Promise<void> => {
    await grid.init();
});
</script>

<template>
    <div>
        <div class="space-y-6">
            <h1 class="text-2xl font-bold text-gray-900">
                Tenants
            </h1>

            <FilterPanel
                storage-key="platform-tenants"
                :has-active-filters="grid.hasActiveFilters.value"
                @clear="grid.clearFilters"
            >
                <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                    <Input
                        v-model="grid.filters.name"
                        label="Tenant"
                        placeholder="Search tenant"
                        flat
                        size="sm"
                        :active="grid.filters.name !== ''"
                    />
                    <Select
                        :model-value="grid.filters.plan"
                        label="Plan"
                        :options="planOptions"
                        flat
                        size="sm"
                        :active="grid.filters.plan !== ''"
                        @update:model-value="grid.filters.plan = $event ?? ''"
                    />
                </div>
            </FilterPanel>

            <BulkActionBar
                :count="grid.selectedCount.value"
                :total="grid.total.value"
                :is-all-filtered="grid.isAllFilteredSelected.value"
                :actions="[]"
                @select-all-filtered="grid.selectAllFiltered"
                @clear="grid.clearSelection"
            />

            <div class="bg-white rounded-lg border border-gray-200 overflow-hidden">
                <DataGrid
                    :columns="columns"
                    :sort="grid.sort.value"
                    :is-loading="grid.isLoading.value"
                    selectable
                    :all-selected="allPageSelected"
                    @sort="grid.handleSort"
                    @toggle-page="grid.togglePage(pageIds)"
                >
                    <template #rows>
                        <PlatformTenantRow
                            v-for="tenant in tenants"
                            :key="tenant.id"
                            :tenant="tenant"
                            selectable
                            :selected="grid.isSelected(tenant.id)"
                            @toggle-select="grid.toggleRow(tenant.id)"
                        />
                        <tr v-if="tenants.length === 0">
                            <td
                                :colspan="columns.length + 1"
                                class="px-4 py-8 text-center text-gray-400"
                            >
                                No tenants
                            </td>
                        </tr>
                    </template>
                </DataGrid>

                <Pagination
                    :page="grid.page.value"
                    :per-page="grid.perPage.value"
                    :total="grid.total.value"
                    @update:page="grid.handlePageChange"
                />
            </div>
        </div>
    </div>
</template>
