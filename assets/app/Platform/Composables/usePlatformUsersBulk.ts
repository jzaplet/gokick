import type { GridState } from '@/app-ui/DataGrid/createGridState';
import type { UsersBulk } from '@/app-ui/BulkActions/createUsersBulk';
import { createUsersBulk } from '@/app-ui/BulkActions/createUsersBulk';
import type { PlatformBulkDeleteUsersRequest } from '@/app/Platform/types/PlatformBulkDeleteUsersRequest';
import type { PlatformBulkActiveUsersRequest } from '@/app/Platform/types/PlatformBulkActiveUsersRequest';

type PlatformGridFilters = {
    tenant: string;
    nickname: string;
    email: string;
    role: string;
    active: string;
};

// The platform users grid's bulk half — the cross-tenant twin: the shared core
// with the platform endpoints and bodies (which carry the tenant filter too).
export const usePlatformUsersBulk = (grid: GridState<PlatformGridFilters>): UsersBulk =>
    createUsersBulk<PlatformGridFilters, PlatformBulkDeleteUsersRequest, PlatformBulkActiveUsersRequest>(
        grid,
        {
            deleteUrl: '/api/v1/platform/users/bulk-delete',
            activeUrl: '/api/v1/platform/users/bulk-active',
            buildDelete: (): PlatformBulkDeleteUsersRequest => ({
                ids: grid.selectedIds(),
                all_filtered: grid.isAllFilteredSelected.value,
                tenant: grid.filters.tenant,
                nickname: grid.filters.nickname,
                email: grid.filters.email,
                role: grid.filters.role,
                active: grid.filters.active,
            }),
            buildActive: (setActive: boolean): PlatformBulkActiveUsersRequest => ({
                ids: grid.selectedIds(),
                all_filtered: grid.isAllFilteredSelected.value,
                tenant: grid.filters.tenant,
                nickname: grid.filters.nickname,
                email: grid.filters.email,
                role: grid.filters.role,
                active: grid.filters.active,
                set_active: setActive,
            }),
            buildActivate: (id: string): PlatformBulkActiveUsersRequest => ({
                ids: [id],
                all_filtered: false,
                tenant: '',
                nickname: '',
                email: '',
                role: '',
                active: '',
                set_active: true,
            }),
        },
    );
