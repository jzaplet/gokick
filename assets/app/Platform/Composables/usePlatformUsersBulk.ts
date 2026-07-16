import type { Ref } from 'vue';
import { ref } from 'vue';
import type { GridState } from '@/app-ui/DataGrid/createGridState';
import type { BulkAction } from '@/app-ui/BulkActions/BulkActionBar.vue';
import type { PlatformBulkDeleteUsersRequest } from '@/app/Platform/types/PlatformBulkDeleteUsersRequest';
import type { PlatformBulkActiveUsersRequest } from '@/app/Platform/types/PlatformBulkActiveUsersRequest';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';

type PlatformGridFilters = {
    tenant: string;
    nickname: string;
    email: string;
    role: string;
    active: string;
};

type PlatformUsersBulk = {
    bulkActions: BulkAction[];
    confirmBulkDelete: Ref<boolean>;
    handleBulkAction: (key: string) => void;
    runBulkDelete: () => Promise<void>;
};

// The bulk half of the platform users grid — the cross-tenant twin of
// useAdminUsersBulk (the payload carries the tenant filter too).
export const usePlatformUsersBulk = (grid: GridState<PlatformGridFilters>): PlatformUsersBulk => {
    const { success, error } = useToast();

    const bulkActions: BulkAction[] = [
        { key: 'activate', label: 'Activate' },
        { key: 'deactivate', label: 'Deactivate' },
        { key: 'delete', label: 'Delete', variant: 'danger' },
    ];

    const confirmBulkDelete = ref(false);

    const runBulkActive = async (setActive: boolean): Promise<void> => {
        const body: PlatformBulkActiveUsersRequest = {
            ids: grid.selectedIds(),
            all_filtered: grid.isAllFilteredSelected.value,
            tenant: grid.filters.tenant,
            nickname: grid.filters.nickname,
            email: grid.filters.email,
            role: grid.filters.role,
            active: grid.filters.active,
            set_active: setActive,
        };
        const result = await authFetch<null, { general?: string }, PlatformBulkActiveUsersRequest>(
            'POST',
            '/api/v1/platform/users/bulk-active',
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

        const body: PlatformBulkDeleteUsersRequest = {
            ids: grid.selectedIds(),
            all_filtered: grid.isAllFilteredSelected.value,
            tenant: grid.filters.tenant,
            nickname: grid.filters.nickname,
            email: grid.filters.email,
            role: grid.filters.role,
            active: grid.filters.active,
        };
        const result = await authFetch<null, { general?: string }, PlatformBulkDeleteUsersRequest>(
            'POST',
            '/api/v1/platform/users/bulk-delete',
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
