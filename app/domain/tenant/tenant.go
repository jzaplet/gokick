package tenant

import (
	"time"

	"github.com/google/uuid"
)

// PlanFree is the default billing tier. gokick ships only the column + this
// default; the product wires the paid tiers, Stripe, and the tenant_usage ledger.
const PlanFree = "free"

// Tenant is the boundary of data ownership — every tenant-owned row belongs to
// exactly one. In single-tenant mode there is just the bootstrap "Default"
// tenant (shared.DefaultTenantID); a multitenant deployment creates one per
// workspace and scopes its data to the tenant id.
type Tenant struct {
	ID   string `db:"id"`
	Name string `db:"name"`
	// Plan is the billing tier (free/paid…). The DB column is NOT NULL DEFAULT
	// 'free'; NewTenant sets it in-memory so a freshly built Tenant matches what
	// the DB stores. Surfaced in the superadmin platform overview.
	Plan      string    `db:"plan"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// Overview is a read model for the superadmin platform plane: a tenant plus a
// count of its users. The cross-tenant aggregate is a single GROUP BY tenant_id
// — trivial under row-level multitenancy — and is the exact shape the product
// reuses to SUM the tenant_usage ledger for per-tenant token spend.
type Overview struct {
	ID        string `db:"id"`
	Name      string `db:"name"`
	Plan      string `db:"plan"`
	UserCount int    `db:"user_count"`
}

// NewTenant builds a tenant with a fresh UUIDv7 id. The bootstrap "Default"
// tenant is created by migration, not this factory.
func NewTenant(name string) *Tenant {
	now := time.Now()
	return &Tenant{
		ID:        uuid.Must(uuid.NewV7()).String(),
		Name:      name,
		Plan:      PlanFree,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
