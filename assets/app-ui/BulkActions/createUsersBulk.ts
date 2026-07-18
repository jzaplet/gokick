import type { ComputedRef, Ref } from 'vue';
import { computed, ref, watch } from 'vue';
import type { GridState } from '@/app-ui/DataGrid/createGridState';
import type { BulkAction } from '@/app-ui/BulkActions/BulkActionBar.vue';
import type { BulkResult } from '@/app-ui/BulkActions/BulkResult';
import { isBulkResult } from '@/app-ui/BulkActions/BulkResult';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';

// The bulk half of a users grid, shared by the admin and platform grids (they
// differ only in endpoint URLs, request-DTO type, and the extra tenant filter —
// all captured by the config below). Bulk actions are Deactivate + Delete;
// activation is a per-row action (activating in bulk is an accident waiting to
// happen). EVERY destructive action is confirmed first, and the server's
// affected count drives a truthful toast (0 rows ≠ success).

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

export type UsersBulk = {
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

// The per-grid differences: where to POST and how to build each request body
// (the platform grid folds its tenant filter into the body). buildActive builds
// both the bulk deactivate payload and the single-id activate payload.
export type UsersBulkConfig<TDeleteBody, TActiveBody> = {
    deleteUrl: string;
    activeUrl: string;
    buildDelete: () => TDeleteBody;
    buildActive: (setActive: boolean) => TActiveBody;
    buildActivate: (id: string) => TActiveBody;
};

export const createUsersBulk = <F extends Record<string, string>, TDeleteBody, TActiveBody>(
    grid: GridState<F>,
    config: UsersBulkConfig<TDeleteBody, TActiveBody>,
): UsersBulk => {
    const { success, info, error } = useToast();

    const bulkActions: BulkAction[] = [
        { key: 'deactivate', label: 'Deactivate' },
        { key: 'delete', label: 'Delete' },
    ];

    const pendingBulk = ref<BulkActionKey | null>(null);

    // A vanishing selection must DISARM, not merely hide. ConfirmModal's visibility
    // is prop-driven, so when a filter change clears the selection out from under
    // the open modal it just disappears — emitting no @cancel, leaving pendingBulk
    // set. The next row ticked would then pop the confirm back up unprompted, over
    // a selection nobody armed, one muscle-memory click from a real deactivate or
    // delete. cancelPendingBulk only covers the explicit Cancel.
    watch(grid.selectedCount, (count: number): void => {
        if (count === 0) {
            pendingBulk.value = null;
        }
    });

    const bulkConfirm = computed((): BulkConfirm | null => {
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
        const body = config.buildActive(false);
        const result = await authFetch<BulkResult, { general?: string }, TActiveBody>(
            'POST',
            config.activeUrl,
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
        const body = config.buildDelete();
        const result = await authFetch<BulkResult, { general?: string }, TDeleteBody>(
            'POST',
            config.deleteUrl,
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

        const body = config.buildActivate(target.id);
        const result = await authFetch<BulkResult, { general?: string }, TActiveBody>(
            'POST',
            config.activeUrl,
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
