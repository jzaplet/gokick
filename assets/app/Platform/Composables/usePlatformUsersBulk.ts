import type { ComputedRef } from 'vue';
import { computed, ref } from 'vue';
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

type BulkActionKey = 'activate' | 'deactivate' | 'delete';

type BulkConfirm = {
    title: string;
    message: string;
    confirmText: string;
};

type PlatformUsersBulk = {
    bulkActions: BulkAction[];
    bulkConfirm: ComputedRef<BulkConfirm | null>;
    handleBulkAction: (key: string) => void;
    runPendingBulk: () => Promise<void>;
    cancelPendingBulk: () => void;
};

// The bulk half of the platform users grid — the cross-tenant twin of
// useAdminUsersBulk (the payload carries the tenant filter too). EVERY bulk
// action goes through the confirm modal — nothing fires until runPendingBulk.
export const usePlatformUsersBulk = (grid: GridState<PlatformGridFilters>): PlatformUsersBulk => {
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
