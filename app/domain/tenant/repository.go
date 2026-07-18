package tenant

import "context"

// Repository is the domain port for tenants — the scoped control-plane methods
// that create-tenant / get-tenant / the seeder need. FindByID returns (nil, nil)
// when the tenant does not exist — the same not-found convention as run.Repository.
// Cross-tenant reads live on PlatformRepository, not here.
type Repository interface {
	Save(ctx context.Context, t *Tenant) error
	FindByID(ctx context.Context, id string) (*Tenant, error)

	// FindByName returns the first tenant with the given name, or (nil, nil) if
	// none — used for idempotent find-or-create (seeder). Name is not unique.
	FindByName(ctx context.Context, name string) (*Tenant, error)
}

// PlatformRepository is the cross-tenant superadmin port for tenant reads — the
// platform plane's overview + dashboard. It embeds Repository and adds the
// *AcrossTenants escape hatches (deliberately unscoped), mirroring
// user.PlatformRepository: segregating them here lets the COMPILER confine
// cross-tenant tenant reads to application/platform, and the *AcrossTenants naming
// also keeps the static isolation gate (zz_platform_isolation_test) covering them.
type PlatformRepository interface {
	Repository

	// OverviewPageAcrossTenants is the paged/filtered/sorted platform tenants grid read: every
	// tenant with its user count (a GROUP BY aggregate), paged by the grid
	// criteria. Cross-tenant by design (platform:overview).
	OverviewPageAcrossTenants(ctx context.Context, criteria ListCriteria) (ListPage, error)

	// CountAcrossTenants returns the total number of tenants (platform dashboard).
	CountAcrossTenants(ctx context.Context) (int, error)

	// DeleteIfEmptyAcrossTenants deletes the tenant only when it still owns
	// nothing live, reporting whether it did. "If empty" is in the name because
	// it is the contract, not an implementation detail: the test and the delete
	// are ONE statement, so a user created between a caller's check and its
	// delete cannot slip through (the grid's count is always stale by the time
	// the click arrives).
	//
	// "Empty" means MORE THAN "no users", and a caller that assumes otherwise
	// will report the wrong reason. A false return means the tenant survived —
	// because it still has users, because it still has unfinished runs, or
	// because it is the default tenant, which is refused by identity and would be
	// refused even when empty. The caller has already established that it exists,
	// so those are the only outcomes; which one it was is NOT knowable from the
	// bool, so do not infer "it still has users" from a false (see
	// DeleteTenantHandler, which names the possibilities instead of choosing).
	DeleteIfEmptyAcrossTenants(ctx context.Context, id string) (bool, error)

	// BulkDeleteEmptyAcrossTenants deletes every SELECTED tenant that owns
	// nothing live (same rule as the single delete, default tenant included) and
	// returns the ids that actually went. Tenants the rule spares are skipped, not
	// an error: the grid lets a superadmin select freely, and partial application
	// is what the returned set is for.
	//
	// It returns ids rather than a count because a count is not recoverable into
	// them: the rows are gone, and in all-filtered mode there is no enumerated
	// selection to diff against. The caller needs the ids for the audit trail of
	// an irreversible cross-tenant delete; len() is the count.
	BulkDeleteEmptyAcrossTenants(ctx context.Context, sel BulkSelection) ([]string, error)
}
