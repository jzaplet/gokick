<script setup lang="ts">
import type { PlatformTenant } from '@/app/Platform/types/PlatformTenant';
import type { PlatformTenantListResponse } from '@/app/Platform/types/PlatformTenantListResponse';
import { isPlatformTenantListResponse } from '@/app/Platform/types/PlatformTenantListResponse';
import type { GridColumn } from '@/app-ui/DataGrid/createGridState';
import { createGridState } from '@/app-ui/DataGrid/createGridState';
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import { usePlatformTenantsBulk } from '@/app/Platform/Composables/usePlatformTenantsBulk';
import DataGrid from '@/app-ui/DataGrid/DataGrid.vue';
import FilterPanel from '@/app-ui/FilterPanel/FilterPanel.vue';
import Pagination from '@/app-ui/Pagination/Pagination.vue';
import Input from '@/app-ui/Inputs/Input.vue';
import Select from '@/app-ui/Inputs/Select.vue';
import Button from '@/app-ui/Buttons/Button.vue';
import PlusIcon from '@/app-ui/Icons/PlusIcon.vue';
import BulkActionBar from '@/app-ui/BulkActions/BulkActionBar.vue';
import ConfirmModal from '@/app-ui/Modals/ConfirmModal.vue';
import PlatformTenantRow from '@/app/Platform/Components/PlatformTenantRow.vue';

const router = useRouter();
const { error } = useToast();

const tenants = ref<PlatformTenant[]>([]);

const columns: GridColumn[] = [
    { key: 'name', label: 'Tenant', sortable: true },
    { key: 'plan', label: 'Plan' },
    { key: 'users', label: 'Users', sortable: true, align: 'right' },
    // The actions column carries no heading (aibobr parity).
    { key: 'actions', label: '', align: 'right' },
];

// Mirrors the backend plan tiers (domain/tenant PlanFree). Only "free" exists
// today; when a paid tier ships (the tenant.go "free/paid…" note), add it here
// so the filter can reach it — the BE matches any plan string exactly.
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

const {
    bulkActions,
    bulkConfirm,
    handleBulkAction,
    runPendingBulk,
    cancelPendingBulk,
    tenantToDelete,
    askDelete,
    cancelDelete,
    runDelete,
} = usePlatformTenantsBulk(grid);

// Every row is selectable, INCLUDING the ones whose delete button is off. The
// user grids exclude rows the server would spare (self, superadmins) because
// there a spared row means the whole action silently did nothing to it. Here
// skipping is the designed outcome — the server deletes the empty tenants in a
// selection and reports how many — so a "select page → delete" that clears the
// empty ones and names the rest is exactly the intended flow.
const pageIds = computed<string[]>(() => tenants.value.map((t) => t.id));

const allPageSelected = computed<boolean>(() =>
    pageIds.value.length > 0
    && pageIds.value.every((id) => grid.isSelected(id) === true));

const goToCreate = (): void => {
    void router.push({ name: 'platform-tenants-new' });
};

onMounted(async (): Promise<void> => {
    await grid.init();
});
</script>

<template>
    <div>
        <div class="space-y-6">
            <div class="flex items-center justify-between gap-4">
                <h1 class="text-2xl font-bold text-gray-900">
                    Platform tenants
                </h1>

                <Button
                    variant="primary"
                    @click="goToCreate"
                >
                    <PlusIcon class="w-4 h-4" />
                    Add tenant
                </Button>
            </div>

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
                :actions="bulkActions"
                @action="handleBulkAction"
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
                            @delete="askDelete"
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

            <ConfirmModal
                :show="tenantToDelete !== null"
                title="Delete tenant"
                :message="tenantToDelete === null
                    ? ''
                    : `Really delete tenant ${tenantToDelete.name}? This action is irreversible.`"
                confirm-text="Delete"
                cancel-text="Cancel"
                @confirm="runDelete"
                @cancel="cancelDelete"
            />

            <ConfirmModal
                :show="bulkConfirm !== null"
                :title="bulkConfirm?.title ?? ''"
                :message="bulkConfirm?.message ?? ''"
                :confirm-text="bulkConfirm?.confirmText ?? 'Confirm'"
                cancel-text="Cancel"
                @confirm="runPendingBulk"
                @cancel="cancelPendingBulk"
            />
        </div>
    </div>
</template>
