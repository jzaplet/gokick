import type { ComputedRef, Ref } from 'vue';
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

// Activation is deliberately NOT a bulk action — it is a per-row action on
// inactive users (aibobr parity), still confirmed and still served by the
// bulk-active endpoint with a single id.
type BulkActionKey = 'deactivate' | 'delete';

type BulkConfirm = {
    title: string;
    message: string;
    confirmText: string;
};

type ActivateTarget = {
    id: string;
    nickname: string;
};

type PlatformUsersBulk = {
    bulkActions: BulkAction[];
    bulkConfirm: ComputedRef<BulkConfirm | null>;
    handleBulkAction: (key: string) => void;
    runPendingBulk: () => Promise<void>;
    cancelPendingBulk: () => void;
    userToActivate: Ref<ActivateTarget | null>;
    askActivate: (user: ActivateTarget) => void;
    cancelActivate: () => void;
    runActivate: () => Promise<void>;
};

// The bulk half of the platform users grid — the cross-tenant twin of
// useAdminUsersBulk (the payload carries the tenant filter too). EVERY
// destructive action is confirmed first — nothing fires until
// runPendingBulk / runActivate.
export const usePlatformUsersBulk = (grid: GridState<PlatformGridFilters>): PlatformUsersBulk => {
    const { success, error } = useToast();

    const bulkActions: BulkAction[] = [
        { key: 'deactivate', label: 'Deactivate' },
        { key: 'delete', label: 'Delete' },
    ];

    const pendingBulk = ref<BulkActionKey | null>(null);

    const bulkConfirm = computed((): BulkConfirm | null => {
        const count = String(grid.selectedCount.value);

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

    const runBulkDeactivate = async (): Promise<void> => {
        const body: PlatformBulkActiveUsersRequest = {
            ids: grid.selectedIds(),
            all_filtered: grid.isAllFilteredSelected.value,
            tenant: grid.filters.tenant,
            nickname: grid.filters.nickname,
            email: grid.filters.email,
            role: grid.filters.role,
            active: grid.filters.active,
            set_active: false,
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

        success('Selected users deactivated.');
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
        if (key === 'deactivate' || key === 'delete') {
            pendingBulk.value = key;
        }
    };

    const runPendingBulk = async (): Promise<void> => {
        const action = pendingBulk.value;

        pendingBulk.value = null;

        if (action === 'delete') {
            await runBulkDelete();
        } else if (action === 'deactivate') {
            await runBulkDeactivate();
        }
    };

    const cancelPendingBulk = (): void => {
        pendingBulk.value = null;
    };

    // Per-row activation: the confirm modal arms here, the endpoint is the
    // bulk-active one narrowed to a single id.
    const userToActivate = ref<ActivateTarget | null>(null);

    const askActivate = (user: ActivateTarget): void => {
        userToActivate.value = user;
    };

    const cancelActivate = (): void => {
        userToActivate.value = null;
    };

    const runActivate = async (): Promise<void> => {
        if (userToActivate.value === null) {
            return;
        }

        const target = userToActivate.value;

        userToActivate.value = null;

        const body: PlatformBulkActiveUsersRequest = {
            ids: [target.id],
            all_filtered: false,
            tenant: '',
            nickname: '',
            email: '',
            role: '',
            active: '',
            set_active: true,
        };
        const result = await authFetch<null, { general?: string }, PlatformBulkActiveUsersRequest>(
            'POST',
            '/api/v1/platform/users/bulk-active',
            { body },
        );

        if (result.success === false) {
            error(result.data.general ?? 'Activation failed.');

            return;
        }

        success(`User ${target.nickname} activated.`);
        await grid.reload();
    };

    return {
        bulkActions,
        bulkConfirm,
        handleBulkAction,
        runPendingBulk,
        cancelPendingBulk,
        userToActivate,
        askActivate,
        cancelActivate,
        runActivate,
    };
};
