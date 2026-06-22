package tenant

import (
	"time"

	"github.com/google/uuid"
)

// Tenant is the boundary of data ownership — every tenant-owned row belongs to
// exactly one. In single-tenant mode there is just the bootstrap "Default"
// tenant (shared.DefaultTenantID); a multitenant deployment creates one per
// workspace and scopes its data to the tenant id.
type Tenant struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// NewTenant builds a tenant with a fresh UUIDv7 id. The bootstrap "Default"
// tenant is created by migration, not this factory.
func NewTenant(name string) *Tenant {
	now := time.Now()
	return &Tenant{
		ID:        uuid.NewString(),
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
