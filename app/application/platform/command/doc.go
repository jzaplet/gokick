// Package command holds the superadmin platform plane's writes: everything gated
// by a platform:* permission. Reach is cross-tenant by design — a superadmin acts
// on any tenant — which is why the repository ports these handlers take are the
// segregated *AcrossTenants ones (user.PlatformRepository, tenant.PlatformRepository)
// that zz_platform_isolation_test confines to this package.
//
// Naming: the Platform infix disambiguates from an ADMIN twin, and only that.
//
//	CreatePlatformUserCommand   — usercmd.CreateUserCommand exists; the infix says which
//	UpdatePlatformUserCommand   — likewise
//	DeletePlatformUserCommand   — likewise
//	CreateTenantCommand         — no twin: tenants live only on this plane
//	DeleteTenantCommand         — no twin
//	CreateSuperAdminCommand     — no twin
//
// The package name already says "platform", so the infix would be noise anywhere
// it isn't resolving an ambiguity. A tenant command that carried it would imply an
// admin-plane counterpart that does not, and cannot, exist.
//
// Two commands here are also dispatched by the CLI (create-tenant,
// create-superadmin) over the SystemCommandBus, which skips Authorize — the
// operator is trusted and has no JWT. That is a difference of ENTRY POINT, not of
// meaning, so they stay single copies: the CLI and the superadmin plane want the
// same rules for creating a tenant, and a second body would be free to drift from
// the first (the F-023/F-031 lesson that produced application/userwrite).
package command
