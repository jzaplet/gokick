package tenant

import "context"

// Repository is the domain port for tenants. FindByID returns (nil, nil) when
// the tenant does not exist — the same not-found convention as job.Repository.
type Repository interface {
	Save(ctx context.Context, t *Tenant) error
	FindByID(ctx context.Context, id string) (*Tenant, error)
}
