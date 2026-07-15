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
                <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-5">
                    <Input
                        v-model="grid.filters.tenant"
                        label="Tenant"
                        placeholder="Search tenant"
                        size="sm"
                    />
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

            <DataGrid
                :columns="columns"
                :sort="grid.sort.value"
                :is-loading="grid.isLoading.value"
                @sort="grid.handleSort"
            >
                <template #rows>
                    <PlatformUserRow
                        v-for="user in users"
                        :key="user.id"
                        :user="user"
                        @edit="goToEdit"
                        @delete="askDelete"
                    />
                    <tr v-if="users.length === 0">
                        <td
                            :colspan="columns.length"
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
        </div>
    </div>
</template>
