package command

import (
	"context"
	"strings"

	"gokick/app/domain/shared"
	"gokick/app/domain/tenant"
)

// CreateTenantCommand creates a tenant. Operator-facing (CLI / superadmin plane);
// the in-app signup that provisions a tenant per workspace is the product's job.
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
	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		return nil, &shared.ValidationError{Field: "name", Message: "tenant name is required"}
	}

	t := tenant.NewTenant(name)
	if err := h.tenants.Save(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}
