package command

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/tenant"
)

// CreateTenantCommand creates a tenant. Operator-facing, and reached two ways:
// the CLI's create-tenant (SystemCommandBus — operator-trusted, no Authorize)
// and POST /api/v1/platform/tenants (CommandBus — Authorize enforces
// platform:tenants:create, so superadmin only). The in-app signup that
// provisions a tenant per workspace is the product's responsibility.
//
// It is NOT CLIOnly: it used to be, back when the CLI was the only way in. The
// superadmin plane now offers it over HTTP, which is what puts
// platform:tenants:create in the FE-facing registry. One command serves both —
// the CLI/HTTP split is a matter of which bus dispatches it, not of what
// creating a tenant means, and a second copy would be free to drift from this one.
type CreateTenantCommand struct {
	Name string
}

func (CreateTenantCommand) RequiredPermission() string { return "platform:tenants:create" }

type CreateTenantHandler struct {
	tenants tenant.Repository
}

func NewCreateTenantHandler(tenants tenant.Repository) *CreateTenantHandler {
	return &CreateTenantHandler{tenants: tenants}
}

func (h *CreateTenantHandler) Handle(
	ctx context.Context,
	cmd CreateTenantCommand,
) (*tenant.Tenant, error) {
	name, err := tenant.NewName(cmd.Name)
	if err != nil {
		return nil, err
	}

	// A tenant's name is how an operator tells tenants apart — the grid lists by
	// it and the Add-user picker offers {id, name}, so two "Acme"s are two
	// identical, unpickable options and filing a user into the wrong one is not
	// fixable in place (an edit never moves a user between tenants). The UNIQUE
	// index is the floor; this check is what turns hitting it into a 400 on the
	// field the form owns instead of a constraint violation surfacing as a 500.
	//
	// Deliberately a check-then-act: two concurrent creates of the same name can
	// both pass it, and then the index refuses the loser. That is the right way
	// round — the check is for the operator's error message, the index is for
	// correctness, and neither is pretending to be the other.
	existing, err := h.tenants.FindByName(ctx, string(name))
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, &shared.ValidationError{
			Field:   "name",
			Message: "a tenant with this name already exists",
		}
	}

	t := tenant.NewTenant(name)
	if err := h.tenants.Save(ctx, t); err != nil {
		return nil, err
	}

	// Through the SystemCommandBus this persists the audit trail of creating a
	// tenant; outside the bus the collector is a throwaway (no-op).
	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     "tenant.created",
		TargetType: "tenant",
		TargetID:   t.ID,
		Metadata:   map[string]any{"name": t.Name},
	})

	return t, nil
}
