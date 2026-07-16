<script setup lang="ts">
import type { PlatformTenant } from '@/app/Platform/types/PlatformTenant';
import type { PlatformTenantListResponse } from '@/app/Platform/types/PlatformTenantListResponse';
import { isPlatformTenantListResponse } from '@/app/Platform/types/PlatformTenantListResponse';
import type { GridColumn } from '@/app-ui/DataGrid/createGridState';
import { createGridState } from '@/app-ui/DataGrid/createGridState';
import { onMounted, ref } from 'vue';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import DataGrid from '@/app-ui/DataGrid/DataGrid.vue';
import FilterPanel from '@/app-ui/FilterPanel/FilterPanel.vue';
import Pagination from '@/app-ui/Pagination/Pagination.vue';
import Input from '@/app-ui/Inputs/Input.vue';
import PlatformTenantRow from '@/app/Platform/Components/PlatformTenantRow.vue';

const { error } = useToast();

const tenants = ref<PlatformTenant[]>([]);

const columns: GridColumn[] = [
    { key: 'name', label: 'Tenant', sortable: true },
    { key: 'plan', label: 'Plan' },
    { key: 'users', label: 'Users', sortable: true, align: 'right' },
];

const grid = createGridState({
    defaultSort: { column: 'name', direction: 'ASC' },
    filters: { name: '' },
    syncUrl: true,
    load: async ({ page, perPage, sort, filters }) => {
        const params = new URLSearchParams({
            page: String(page),
            per_page: String(perPage),
            sort_by: sort.column,
            sort_dir: sort.direction,
        });

        if (filters.name !== '') {
            params.set('name', filters.name);
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

onMounted(async (): Promise<void> => {
    await grid.init();
});
</script>

<template>
    <div class="py-12 px-4 sm:px-6 lg:px-8">
        <div class="max-w-5xl mx-auto space-y-6">
            <h1 class="text-3xl font-extrabold text-gray-900">
                Tenants
            </h1>

            <FilterPanel
                storage-key="platform-tenants"
                :has-active-filters="grid.hasActiveFilters.value"
                @clear="grid.clearFilters"
            >
                <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
                    <Input
                        v-model="grid.filters.name"
                        label="Tenant"
                        placeholder="Search tenant"
                        flat
                        size="sm"
                    />
                </div>
            </FilterPanel>

            <DataGrid
                :columns="columns"
                :sort="grid.sort.value"
                :is-loading="grid.isLoading.value"
                @sort="grid.handleSort"
            >
                <template #rows>
                    <PlatformTenantRow
                        v-for="tenant in tenants"
                        :key="tenant.id"
                        :tenant="tenant"
                    />
                    <tr v-if="tenants.length === 0">
                        <td
                            :colspan="columns.length"
                            class="px-6 py-8 text-center text-sm text-gray-500"
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
</template>
