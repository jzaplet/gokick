package bus

// SystemCommandBus is the write-side bus for OPERATOR-TRUSTED commands that run
// OUTSIDE an HTTP request — the CLI create-* commands (and, later, seed). It
// assembles a SUBSET of the regular CommandBus chain:
//
//	Recovery → Logging → Audit → DispatchEvents → Transaction
//
// deliberately OMITTING Authorize and Tenant:
//   - Authorize: there is no authenticated principal — the operator is trusted by
//     virtue of shell access, so the permission check would be theatre (and would
//     fail, since there are no claims in ctx).
//   - Tenant: there is no JWT to resolve a tenant from. These commands inject the
//     tenant explicitly (create-user --tenant-*) or operate on the default /
//     cross-tenant plane (create-superadmin); TenantMiddleware would overwrite the
//     injected value with the default resolver's fallback.
//
// What the chain DOES give the CLI, which a bare handler call does not: one
// transaction (multi-write commands are atomic — the orphan-tenant rollback comes
// for free instead of being hand-rolled), an audit trail, post-commit event
// dispatch, and panic→Sentry reporting.
// Its inner *Bus is unexported: an operator command runs only through
// bus.SystemDispatch / bus.SystemDispatchVoid, which take *SystemCommandBus (see
// CommandBus for the compile-checked-pairing rationale).
type SystemCommandBus struct{ inner *Bus }

func NewSystemCommandBus(middlewares ...Middleware) *SystemCommandBus {
	return &SystemCommandBus{inner: newBus(middlewares...)}
}
