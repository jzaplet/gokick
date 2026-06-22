package tenant

import "context"

// Repository is the domain port for tenants. FindByID returns (nil, nil) when
// the tenant does not exist — the same not-found convention as job.Repository.
type Repository interface {
	Save(ctx context.Context, t *Tenant) error
	FindByID(ctx context.Context, id string) (*Tenant, error)

	// FindByName returns the first tenant with the given name, or (nil, nil) if
	// none — used for idempotent find-or-create (seeder). Name is not unique.
	FindByName(ctx context.Context, name string) (*Tenant, error)

	// FindAllWithUserCount returns every tenant with its user count, for the
	// superadmin platform overview (platform:overview). Cross-tenant by design.
	FindAllWithUserCount(ctx context.Context) ([]Overview, error)

	// Count returns the total number of tenants (platform dashboard).
	Count(ctx context.Context) (int, error)
}
