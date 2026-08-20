import type { ApiMessage } from '@/app-ui/Fetch/types/ApiMessage';

// Platform's own copy of the user-form error shape — deliberately independent of
// Admin's UserFormErrors so the platform plane owns its user management without a
// cross-domain import (F-084, direction B). Hand-written: the backend returns
// per-field validation errors keyed by these names, no field is codegen-driven.
// Values are wire messages ({ key, params }) rendered via tm() at display time.
// tenant_id is create-only: the platform create resolves the chosen tenant and
// reports a missing/unknown one against that field
// (CreatePlatformUserHandler.resolveTenant). The edit form never renders it.
export type PlatformUserFormErrors = {
    general?: ApiMessage;
    nickname?: ApiMessage;
    password?: ApiMessage;
    email?: ApiMessage;
    role?: ApiMessage;
    tenant_id?: ApiMessage;
};
