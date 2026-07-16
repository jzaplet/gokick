import type { Ref } from 'vue';
import { ref } from 'vue';
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

type AdminUsersBulk = {
    bulkActions: BulkAction[];
    confirmBulkDelete: Ref<boolean>;
    handleBulkAction: (key: string) => void;
    runBulkDelete: () => Promise<void>;
};

// The bulk half of the admin users grid: builds the dual-mode payload (ids, or
// all_filtered + the CURRENT filter set) for the two bulk endpoints, confirms
// deletes, and resets selection + reloads after success. Extracted from the
// view — views stay orchestrators.
export const useAdminUsersBulk = (grid: GridState<AdminGridFilters>): AdminUsersBulk => {
    const { success, error } = useToast();

    const bulkActions: BulkAction[] = [
        { key: 'activate', label: 'Activate' },
        { key: 'deactivate', label: 'Deactivate' },
        { key: 'delete', label: 'Delete' },
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

    return { bulkActions, confirmBulkDelete, handleBulkAction, runBulkDelete };
};
