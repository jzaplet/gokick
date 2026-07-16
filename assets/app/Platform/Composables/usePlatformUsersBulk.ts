import type { ComputedRef, Ref } from 'vue';
import { computed, ref } from 'vue';
import type { GridState } from '@/app-ui/DataGrid/createGridState';
import type { BulkAction } from '@/app-ui/BulkActions/BulkActionBar.vue';
import type { PlatformBulkDeleteUsersRequest } from '@/app/Platform/types/PlatformBulkDeleteUsersRequest';
import type { PlatformBulkActiveUsersRequest } from '@/app/Platform/types/PlatformBulkActiveUsersRequest';
import type { BulkResult } from '@/app-ui/BulkActions/BulkResult';
import { isBulkResult } from '@/app-ui/BulkActions/BulkResult';
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
    const { success, info, error } = useToast();

    const bulkActions: BulkAction[] = [
        { key: 'deactivate', label: 'Deactivate' },
        { key: 'delete', label: 'Delete' },
    ];

    const pendingBulk = ref<BulkActionKey | null>(null);

    const bulkConfirm = computed((): BulkConfirm | null => {
        // Selection vanished under the open modal (a debounced filter change
        // cleared it): close rather than confirm an empty operation.
        if (grid.selectedCount.value === 0) {
            return null;
        }

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

    // The server reports the actual affected count — 0 means the selection
    // collapsed to rows it spared (superadmins, the actor) or ids that no
    // longer exist, so a success toast would lie.
    const reportBulk = (affected: number, verb: string): void => {
        if (affected === 0) {
            info('No users were changed.');

            return;
        }

        success(`${String(affected)} user(s) ${verb}.`);
    };

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
        const result = await authFetch<BulkResult, { general?: string }, PlatformBulkActiveUsersRequest>(
            'POST',
            '/api/v1/platform/users/bulk-active',
            { body, validate: isBulkResult },
        );

        if (result.success === false) {
            error(result.data.general ?? 'Bulk update failed.');

            return;
        }

        reportBulk(result.data.affected, 'deactivated');
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
        const result = await authFetch<BulkResult, { general?: string }, PlatformBulkDeleteUsersRequest>(
            'POST',
            '/api/v1/platform/users/bulk-delete',
            { body, validate: isBulkResult },
        );

        if (result.success === false) {
            error(result.data.general ?? 'Bulk delete failed.');

            return;
        }

        reportBulk(result.data.affected, 'deleted');
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

        // The selection may have cleared under the modal (debounced filter
        // change) between arming and confirming — do nothing rather than post
        // an empty payload that the backend rejects.
        if (grid.selectedCount.value === 0) {
            return;
        }

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
        const result = await authFetch<BulkResult, { general?: string }, PlatformBulkActiveUsersRequest>(
            'POST',
            '/api/v1/platform/users/bulk-active',
            { body, validate: isBulkResult },
        );

        if (result.success === false) {
            error(result.data.general ?? 'Activation failed.');

            return;
        }

        if (result.data.affected === 0) {
            info('No change — the user may no longer exist.');
        } else {
            success(`User ${target.nickname} activated.`);
        }
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
