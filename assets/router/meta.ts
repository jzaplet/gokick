/* eslint-disable @typescript-eslint/consistent-type-definitions --
   Module augmentation requires `interface` for declaration merging. Using
   `type` here would REPLACE vue-router's RouteMeta instead of extending it.
   Scoped to this file so the "no interface" rule still applies everywhere
   else in the codebase. */

import type { RouteMeta, RouteRecordRaw } from 'vue-router';
import type { Permission } from '@/app/Auth/enums/resources';

declare module 'vue-router' {
    interface RouteMeta {
        requiresAuth: boolean;
        requiresPermission?: Permission;
    }
}

// AppRoute forces every entry in the routes array to include meta. Without
// this intersection vue-router's `meta?: RouteMeta` would let callers omit
// meta entirely and silently default to "public".
export type AppRoute = RouteRecordRaw & { meta: RouteMeta };
