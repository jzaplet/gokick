// The tenant-form error shape. Hand-written, mirroring PlatformUserFormErrors:
// the backend returns per-field validation errors keyed by these names, and no
// field here is codegen-driven (tsgen emits the REQUEST shape, not the error one).
//
// `name` is the only field a tenant has today; `general` catches everything the
// Responder could not route to a field (see the FieldError contract).
export type PlatformTenantFormErrors = {
    general?: string;
    name?: string;
};
