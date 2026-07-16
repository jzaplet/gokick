<script setup lang="ts">
import type { PlatformUser } from '@/app/Platform/types/PlatformUser';
import type { PlatformUserListResponse } from '@/app/Platform/types/PlatformUserListResponse';
import { isPlatformUserListResponse } from '@/app/Platform/types/PlatformUserListResponse';
import type { GridColumn } from '@/app-ui/DataGrid/createGridState';
import { createGridState } from '@/app-ui/DataGrid/createGridState';
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import ConfirmModal from '@/app-ui/Modals/ConfirmModal.vue';
import DataGrid from '@/app-ui/DataGrid/DataGrid.vue';
import FilterPanel from '@/app-ui/FilterPanel/FilterPanel.vue';
import Pagination from '@/app-ui/Pagination/Pagination.vue';
import Input from '@/app-ui/Inputs/Input.vue';
import Select from '@/app-ui/Inputs/Select.vue';
import PlatformUserRow from '@/app/Platform/Components/PlatformUserRow.vue';
import BulkActionBar from '@/app-ui/BulkActions/BulkActionBar.vue';
import { usePlatformUsersBulk } from '@/app/Platform/Composables/usePlatformUsersBulk';
import { Role } from '@/app/Auth/enums/roles';
import { computed } from 'vue';

const router = useRouter();
const { success, error } = useToast();

const users = ref<PlatformUser[]>([]);
const userToDelete = ref<PlatformUser | null>(null);

const columns: GridColumn[] = [
    { key: 'tenant', label: 'Tenant', sortable: true },
    { key: 'nickname', label: 'Nickname', sortable: true },
    { key: 'email', label: 'Email', sortable: true },
    { key: 'role', label: 'Role', sortable: true },
    { key: 'last_login', label: 'Last login', sortable: true },
    { key: 'actions', label: 'Actions', align: 'right' },
];

const roleOptions = [
    { value: '', label: 'All roles' },
    { value: 'superadmin', label: 'superadmin' },
    { value: 'admin', label: 'admin' },
    { value: 'user', label: 'user' },
];

const activeOptions = [
    { value: '', label: 'All statuses' },
    { value: '1', label: 'Active' },
    { value: '0', label: 'Inactive' },
];

const grid = createGridState({
    defaultSort: { column: 'tenant', direction: 'ASC' },
    filters: { tenant: '', nickname: '', email: '', role: '', active: '' },
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

        const result = await authFetch<PlatformUserListResponse>(
            'GET',
            `/api/v1/platform/users?${params.toString()}`,
            { validate: isPlatformUserListResponse },
        );

        if (result.success === false) {
            error('Failed to load user list.');

            return { ok: false };
        }

        users.value = result.data.items;

        return { ok: true, total: result.data.total };
    },
});

const goToEdit = (user: PlatformUser): void => {
    void router.push({ name: 'platform-users-edit', params: { id: user.id } });
};

const askDelete = (user: PlatformUser): void => {
    userToDelete.value = user;
};

const cancelDelete = (): void => {
    userToDelete.value = null;
};

const confirmDelete = async (): Promise<void> => {
    if (userToDelete.value === null) {
        return;
    }

    const target = userToDelete.value;

    userToDelete.value = null;

    const result = await authFetch<null, { general?: string }>(
        'DELETE',
        `/api/v1/platform/users/${target.id}`,
    );

    if (result.success === false) {
        error(result.data.general ?? 'Delete failed.');

        return;
    }

    success(`User ${target.nickname} deleted.`);
    await grid.reload();
};

// Selection: superadmin rows are never selectable (the BE spares them and
// the actor too), so "page selected" means every manageable row on the page.
const pageSelectableIds = computed<string[]>(() =>
    users.value
        .filter((u) => u.role !== Role.SuperAdmin)
        .map((u) => u.id));

const allPageSelected = computed<boolean>(() =>
    pageSelectableIds.value.length > 0
    && pageSelectableIds.value.every((id) => grid.isSelected(id) === true));

const {
    bulkActions,
    confirmBulkDelete,
    handleBulkAction,
    runBulkDelete,
} = usePlatformUsersBulk(grid);

onMounted(async (): Promise<void> => {
    await grid.init();
});
</script>

<template>
    <div class="py-12 px-4 sm:px-6 lg:px-8">
        <div class="max-w-6xl mx-auto space-y-6">
            <h1 class="text-3xl font-extrabold text-gray-900">
                Platform users
            </h1>

            <FilterPanel
                storage-key="platform-users"
                :has-active-filters="grid.hasActiveFilters.value"
                @clear="grid.clearFilters"
            >
                <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
                    <Input
                        v-model="grid.filters.tenant"
                        label="Tenant"
                        placeholder="Search tenant"
                        flat
                        size="sm"
                        :active="grid.filters.tenant !== ''"
                    />
                    <Input
                        v-model="grid.filters.nickname"
                        label="Nickname"
                        placeholder="Search nickname"
                        flat
                        size="sm"
                        :active="grid.filters.nickname !== ''"
                    />
                    <Input
                        v-model="grid.filters.email"
                        label="Email"
                        placeholder="Search email"
                        flat
                        size="sm"
                        :active="grid.filters.email !== ''"
                    />
                    <Select
                        :model-value="grid.filters.role"
                        label="Role"
                        :options="roleOptions"
                        flat
                        size="sm"
                        :active="grid.filters.role !== ''"
                        @update:model-value="grid.filters.role = $event ?? ''"
                    />
                    <Select
                        :model-value="grid.filters.active"
                        label="Status"
                        :options="activeOptions"
                        flat
                        size="sm"
                        :active="grid.filters.active !== ''"
                        @update:model-value="grid.filters.active = $event ?? ''"
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
                    @toggle-page="grid.togglePage(pageSelectableIds)"
                >
                    <template #rows>
                        <PlatformUserRow
                            v-for="user in users"
                            :key="user.id"
                            :user="user"
                            selectable
                            :selected="grid.isSelected(user.id)"
                            @edit="goToEdit"
                            @delete="askDelete"
                            @toggle-select="grid.toggleRow(user.id)"
                        />
                        <tr v-if="users.length === 0">
                            <td
                                :colspan="columns.length + 1"
                                class="px-4 py-8 text-center text-gray-400"
                            >
                                No users
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
                :show="userToDelete !== null"
                title="Delete user"
                :message="userToDelete === null
                    ? ''
                    : `Really delete user ${userToDelete.nickname}? This action is irreversible.`"
                confirm-text="Delete"
                cancel-text="Cancel"
                @confirm="confirmDelete"
                @cancel="cancelDelete"
            />

            <ConfirmModal
                :show="confirmBulkDelete === true"
                title="Delete selected users"
                :message="`Really delete ${String(grid.selectedCount.value)} selected user(s)?
                    This action is irreversible.`"
                confirm-text="Delete"
                cancel-text="Cancel"
                @confirm="runBulkDelete"
                @cancel="confirmBulkDelete = false"
            />
        </div>
    </div>
</template>
