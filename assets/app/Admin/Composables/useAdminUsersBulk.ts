import type { ComputedRef } from 'vue';
import { computed, ref } from 'vue';
import type { GridState } from '@/app-ui/DataGrid/createGridState';
import type { BulkAction } from '@/app-ui/BulkActions/BulkActionBar.vue';
import type { BulkDeleteUsersRequest } from '@/app/Admin/types/BulkDeleteUsersRequest';
import type { BulkActiveUsersRequest } from '@/app/Admin/types/BulkActiveUsersRequest';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';

type AdminGridFilters = {
    nickname: string;
    email: string;
    role: string;
    active: string;
};

type BulkActionKey = 'activate' | 'deactivate' | 'delete';

type BulkConfirm = {
    title: string;
    message: string;
    confirmText: string;
};

type AdminUsersBulk = {
    bulkActions: BulkAction[];
    bulkConfirm: ComputedRef<BulkConfirm | null>;
    handleBulkAction: (key: string) => void;
    runPendingBulk: () => Promise<void>;
    cancelPendingBulk: () => void;
};

// The bulk half of the admin users grid: builds the dual-mode payload (ids, or
// all_filtered + the CURRENT filter set) for the two bulk endpoints, and
// resets selection + reloads after success. EVERY bulk action is destructive
// at scale, so each one goes through the confirm modal — nothing fires until
// runPendingBulk. Extracted from the view — views stay orchestrators.
export const useAdminUsersBulk = (grid: GridState<AdminGridFilters>): AdminUsersBulk => {
    const { success, error } = useToast();

    const bulkActions: BulkAction[] = [
        { key: 'activate', label: 'Activate' },
        { key: 'deactivate', label: 'Deactivate' },
        { key: 'delete', label: 'Delete' },
    ];

    const pendingBulk = ref<BulkActionKey | null>(null);

    const bulkConfirm = computed((): BulkConfirm | null => {
        const count = String(grid.selectedCount.value);

        if (pendingBulk.value === 'activate') {
            return {
                title: 'Activate selected users',
                message: `Activate ${count} selected user(s)?`,
                confirmText: 'Activate',
            };
        }
        if (pendingBulk.value === 'deactivate') {
            return {
                title: 'Deactivate selected users',
                message: `Deactivate ${count} selected user(s)?`,
                confirmText: 'Deactivate',
            };
        }
        if (pendingBulk.value === 'delete') {
            return {
                title: 'Delete selected users',
                message: `Really delete ${count} selected user(s)? This action is irreversible.`,
                confirmText: 'Delete',
            };
        }

        return null;
    });

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
        if (key === 'activate' || key === 'deactivate' || key === 'delete') {
            pendingBulk.value = key;
        }
    };

    const runPendingBulk = async (): Promise<void> => {
        const action = pendingBulk.value;

        pendingBulk.value = null;

        if (action === 'delete') {
            await runBulkDelete();
        } else if (action === 'activate') {
            await runBulkActive(true);
        } else if (action === 'deactivate') {
            await runBulkActive(false);
        }
    };

    const cancelPendingBulk = (): void => {
        pendingBulk.value = null;
    };

    return { bulkActions, bulkConfirm, handleBulkAction, runPendingBulk, cancelPendingBulk };
};
