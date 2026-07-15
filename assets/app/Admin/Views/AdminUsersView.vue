<script setup lang="ts">
import type { AdminUser } from '@/app/Admin/types/AdminUser';
import type { AdminUserListResponse } from '@/app/Admin/types/AdminUserListResponse';
import { isAdminUserListResponse } from '@/app/Admin/types/AdminUserListResponse';
import type { GridColumn } from '@/app-ui/DataGrid/createGridState';
import { createGridState } from '@/app-ui/DataGrid/createGridState';
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import Button from '@/app-ui/Buttons/Button.vue';
import PlusIcon from '@/app-ui/Icons/PlusIcon.vue';
import ConfirmModal from '@/app-ui/Modals/ConfirmModal.vue';
import DataGrid from '@/app-ui/DataGrid/DataGrid.vue';
import FilterPanel from '@/app-ui/FilterPanel/FilterPanel.vue';
import Pagination from '@/app-ui/Pagination/Pagination.vue';
import Input from '@/app-ui/Inputs/Input.vue';
import Select from '@/app-ui/Inputs/Select.vue';
import AdminUserRow from '@/app/Admin/Components/AdminUserRow.vue';
import BulkActionBar from '@/app-ui/BulkActions/BulkActionBar.vue';
import type { BulkAction } from '@/app-ui/BulkActions/BulkActionBar.vue';
import type { BulkDeleteUsersRequest } from '@/app/Admin/types/BulkDeleteUsersRequest';
import type { BulkActiveUsersRequest } from '@/app/Admin/types/BulkActiveUsersRequest';
import { useAuth } from '@/app-ui/Auth';
import { computed } from 'vue';

const router = useRouter();
const { success, error } = useToast();

const users = ref<AdminUser[]>([]);
const userToDelete = ref<AdminUser | null>(null);

const columns: GridColumn[] = [
    { key: 'nickname', label: 'Nickname', sortable: true },
    { key: 'email', label: 'Email', sortable: true },
    { key: 'role', label: 'Role', sortable: true },
    { key: 'actions', label: 'Actions', align: 'right' },
];

const roleOptions = [
    { value: '', label: 'All roles' },
    { value: 'admin', label: 'admin' },
    { value: 'user', label: 'user' },
];

const activeOptions = [
    { value: '', label: 'All statuses' },
    { value: '1', label: 'Active' },
    { value: '0', label: 'Inactive' },
];

const grid = createGridState({
    defaultSort: { column: 'nickname', direction: 'ASC' },
    filters: { nickname: '', email: '', role: '', active: '' },
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

        const result = await authFetch<AdminUserListResponse>(
            'GET',
            `/api/v1/admin/users?${params.toString()}`,
            { validate: isAdminUserListResponse },
        );

        if (result.success === false) {
            error('Failed to load user list.');

            return { ok: false };
        }

        users.value = result.data.items;

        return { ok: true, total: result.data.total };
    },
});

const goToCreate = (): void => {
    void router.push({ name: 'admin-users-new' });
};

const goToEdit = (user: AdminUser): void => {
    void router.push({ name: 'admin-users-edit', params: { id: user.id } });
};

const askDelete = (user: AdminUser): void => {
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
        `/api/v1/admin/users/${target.id}`,
    );

    if (result.success === false) {
        error(result.data.general ?? 'Delete failed.');

        return;
    }

    success(`User ${target.nickname} deleted.`);
    await grid.reload();
};

const { user: currentUser } = useAuth();

// Selection: the actor's own row is never selectable (bulk ops exclude the
// actor on the BE too), so "page selected" means every OTHER row on the page.
const pageSelectableIds = computed<string[]>(() =>
    users.value
        .filter((u) => u.id !== currentUser.value?.id)
        .map((u) => u.id));

const allPageSelected = computed<boolean>(() =>
    pageSelectableIds.value.length > 0
    && pageSelectableIds.value.every((id) => grid.isSelected(id) === true));

const bulkActions: BulkAction[] = [
    { key: 'activate', label: 'Activate' },
    { key: 'deactivate', label: 'Deactivate' },
    { key: 'delete', label: 'Delete', variant: 'danger' },
];

const confirmBulkDelete = ref(false);

const runBulkActive = async (setActive: boolean): Promise<void> => {
    const body: BulkActiveUsersRequest = {
        ids: grid.selectedIds(),
        all_filtered: grid.isAllFilteredSelected.value,
        nickname: grid.filters.nickname,
        email: grid.filters.email,
        role: grid.filters.role,
        active: grid.filters.active,
        set_active: setActive,
    };
    const result = await authFetch<null, { general?: string }, BulkActiveUsersRequest>(
        'POST',
        '/api/v1/admin/users/bulk-active',
        { body },
    );

    if (result.success === false) {
        error(result.data.general ?? 'Bulk update failed.');

        return;
    }

    success(setActive === true ? 'Selected users activated.' : 'Selected users deactivated.');
    grid.clearSelection();
    await grid.reload();
};

const runBulkDelete = async (): Promise<void> => {
    confirmBulkDelete.value = false;

    const body: BulkDeleteUsersRequest = {
        ids: grid.selectedIds(),
        all_filtered: grid.isAllFilteredSelected.value,
        nickname: grid.filters.nickname,
        email: grid.filters.email,
        role: grid.filters.role,
        active: grid.filters.active,
    };
    const result = await authFetch<null, { general?: string }, BulkDeleteUsersRequest>(
        'POST',
        '/api/v1/admin/users/bulk-delete',
        { body },
    );

    if (result.success === false) {
        error(result.data.general ?? 'Bulk delete failed.');

        return;
    }

    success('Selected users deleted.');
    grid.clearSelection();
    await grid.reload();
};

const handleBulkAction = (key: string): void => {
    if (key === 'delete') {
        confirmBulkDelete.value = true;
    } else if (key === 'activate') {
        void runBulkActive(true);
    } else if (key === 'deactivate') {
        void runBulkActive(false);
    }
};

onMounted(async (): Promise<void> => {
    await grid.init();
});
</script>

<template>
    <div class="py-12 px-4 sm:px-6 lg:px-8">
        <div class="max-w-5xl mx-auto space-y-6">
            <div
                :class="[
                    'flex flex-col gap-4',
                    'sm:flex-row sm:items-center sm:justify-between',
                ]"
            >
                <h1 class="text-3xl font-extrabold text-gray-900">
                    User management
                </h1>

                <Button
                    variant="primary"
                    @click="goToCreate"
                >
                    <PlusIcon class="w-4 h-4" />
                    Add user
                </Button>
            </div>

            <FilterPanel
                storage-key="admin-users"
                :has-active-filters="grid.hasActiveFilters.value"
                @clear="grid.clearFilters"
            >
                <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
                    <Input
                        v-model="grid.filters.nickname"
                        label="Nickname"
                        placeholder="Search nickname"
                        size="sm"
                    />
                    <Input
                        v-model="grid.filters.email"
                        label="Email"
                        placeholder="Search email"
                        size="sm"
                    />
                    <Select
                        v-model="grid.filters.role"
                        label="Role"
                        :options="roleOptions"
                        size="sm"
                    />
                    <Select
                        v-model="grid.filters.active"
                        label="Status"
                        :options="activeOptions"
                        size="sm"
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
                    <AdminUserRow
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
                            class="px-6 py-8 text-center text-sm text-gray-500"
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
