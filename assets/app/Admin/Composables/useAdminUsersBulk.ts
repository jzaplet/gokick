import type { ComputedRef, Ref } from 'vue';
import { computed, ref } from 'vue';
import type { GridState } from '@/app-ui/DataGrid/createGridState';
import type { BulkAction } from '@/app-ui/BulkActions/BulkActionBar.vue';
import type { BulkDeleteUsersRequest } from '@/app/Admin/types/BulkDeleteUsersRequest';
import type { BulkActiveUsersRequest } from '@/app/Admin/types/BulkActiveUsersRequest';
import type { BulkResult } from '@/app-ui/BulkActions/BulkResult';
import { isBulkResult } from '@/app-ui/BulkActions/BulkResult';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';

type AdminGridFilters = {
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

type AdminUsersBulk = {
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

// The bulk half of the admin users grid: builds the dual-mode payload (ids, or
// all_filtered + the CURRENT filter set) for the two bulk endpoints, and
// resets selection + reloads after success. EVERY destructive action is
// confirmed first — nothing fires until runPendingBulk / runActivate.
// Extracted from the view — views stay orchestrators.
export const useAdminUsersBulk = (grid: GridState<AdminGridFilters>): AdminUsersBulk => {
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
    // collapsed to rows it spared (the actor, superadmins) or ids that no
    // longer exist, so a success toast would lie.
    const reportBulk = (affected: number, verb: string): void => {
        if (affected === 0) {
            info('No users were changed.');

            return;
        }

        success(`${String(affected)} user(s) ${verb}.`);
    };

    const runBulkDeactivate = async (): Promise<void> => {
        const body: BulkActiveUsersRequest = {
            ids: grid.selectedIds(),
            all_filtered: grid.isAllFilteredSelected.value,
            nickname: grid.filters.nickname,
            email: grid.filters.email,
            role: grid.filters.role,
            active: grid.filters.active,
            set_active: false,
        };
        const result = await authFetch<BulkResult, { general?: string }, BulkActiveUsersRequest>(
            'POST',
            '/api/v1/admin/users/bulk-active',
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
        const body: BulkDeleteUsersRequest = {
            ids: grid.selectedIds(),
            all_filtered: grid.isAllFilteredSelected.value,
            nickname: grid.filters.nickname,
            email: grid.filters.email,
            role: grid.filters.role,
            active: grid.filters.active,
        };
        const result = await authFetch<BulkResult, { general?: string }, BulkDeleteUsersRequest>(
            'POST',
            '/api/v1/admin/users/bulk-delete',
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

        const body: BulkActiveUsersRequest = {
            ids: [target.id],
            all_filtered: false,
            nickname: '',
            email: '',
            role: '',
            active: '',
            set_active: true,
        };
        const result = await authFetch<BulkResult, { general?: string }, BulkActiveUsersRequest>(
            'POST',
            '/api/v1/admin/users/bulk-active',
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
