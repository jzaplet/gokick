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

	// DeleteIfEmptyAcrossTenants deletes the tenant only when it still owns no
	// users, reporting whether it did. "If empty" is in the name because it is
	// the contract, not an implementation detail: the emptiness test and the
	// delete are ONE statement, so a user created between a caller's check and
	// its delete cannot slip through (the grid's count is always stale by the
	// time the click arrives). A false return means the tenant survived because
	// it still has users — the caller has already established that it exists.
	DeleteIfEmptyAcrossTenants(ctx context.Context, id string) (bool, error)

	// BulkDeleteEmptyAcrossTenants deletes every SELECTED tenant that owns no
	// users and returns how many went. Non-empty tenants in the selection are
	// skipped, not an error: the grid lets a superadmin select freely, and
	// partial application is what the affected count is for.
	BulkDeleteEmptyAcrossTenants(ctx context.Context, sel BulkSelection) (int64, error)
}
