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

	// OverviewAcrossTenants returns every tenant with its user count, for the
	// superadmin platform overview (platform:overview). Cross-tenant by design.
	OverviewAcrossTenants(ctx context.Context) ([]Overview, error)

	// OverviewPage is the paged/filtered/sorted platform tenants grid read —
	// OverviewAcrossTenants' aggregate with the grid criteria shape.
	OverviewPage(ctx context.Context, criteria ListCriteria) (ListPage, error)

	// CountAcrossTenants returns the total number of tenants (platform dashboard).
	CountAcrossTenants(ctx context.Context) (int, error)
}
