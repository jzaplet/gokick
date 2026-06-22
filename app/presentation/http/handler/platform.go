package handler

import (
	"context"
	"net/http"
	"time"

	"gokick/app/application/bus"
	platformqry "gokick/app/application/platform/query"
	"gokick/app/domain/tenant"
	"gokick/app/domain/user"
	"gokick/app/presentation/http/response"
)

// PlatformHandler serves the superadmin platform plane — cross-tenant overviews
// gated behind platform:overview (superadmin only). Read-only; the product fills
// the deeper data (token usage, activity, billing).
type PlatformHandler struct {
	queryBus    *bus.QueryBus
	listUsers   *platformqry.ListAllUsersHandler
	listTenants *platformqry.ListTenantsHandler
}

func NewPlatformHandler(
	queryBus *bus.QueryBus,
	listUsers *platformqry.ListAllUsersHandler,
	listTenants *platformqry.ListTenantsHandler,
) *PlatformHandler {
	return &PlatformHandler{
		queryBus:    queryBus,
		listUsers:   listUsers,
		listTenants: listTenants,
	}
}

type platformUserDTO struct {
	ID          string  `json:"id"`
	Nickname    string  `json:"nickname"`
	Email       string  `json:"email"`
	Role        string  `json:"role"`
	Active      bool    `json:"active"`
	TenantID    string  `json:"tenant_id"`
	LastLoginAt *string `json:"last_login_at"`
}

type platformTenantDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Plan      string `json:"plan"`
	UserCount int    `json:"user_count"`
}

func (h *PlatformHandler) Users(w http.ResponseWriter, r *http.Request) {
	q := platformqry.ListAllUsersQuery{}

	users, err := bus.Exec(
		r.Context(),
		h.queryBus.Bus,
		"PlatformListUsers",
		q,
		func(ctx context.Context) ([]user.User, error) {
			return h.listUsers.Handle(ctx, q)
		},
	)
	if err != nil {
		response.HandleError(w, err)

		return
	}

	dtos := make([]platformUserDTO, len(users))
	for i, u := range users {
		dtos[i] = toPlatformUserDTO(u)
	}

	response.JSON(w, http.StatusOK, dtos)
}

func (h *PlatformHandler) Tenants(w http.ResponseWriter, r *http.Request) {
	q := platformqry.ListTenantsQuery{}

	rows, err := bus.Exec(
		r.Context(),
		h.queryBus.Bus,
		"PlatformListTenants",
		q,
		func(ctx context.Context) ([]tenant.Overview, error) {
			return h.listTenants.Handle(ctx, q)
		},
	)
	if err != nil {
		response.HandleError(w, err)

		return
	}

	dtos := make([]platformTenantDTO, len(rows))
	for i, t := range rows {
		dtos[i] = platformTenantDTO{
			ID:        t.ID,
			Name:      t.Name,
			Plan:      t.Plan,
			UserCount: t.UserCount,
		}
	}

	response.JSON(w, http.StatusOK, dtos)
}

func toPlatformUserDTO(u user.User) platformUserDTO {
	var lastLogin *string
	if u.LastLoginAt.Valid {
		s := u.LastLoginAt.Time.Format(time.RFC3339)
		lastLogin = &s
	}

	return platformUserDTO{
		ID:          u.ID,
		Nickname:    u.Nickname,
		Email:       u.Email,
		Role:        u.Role,
		Active:      u.Active,
		TenantID:    u.TenantID,
		LastLoginAt: lastLogin,
	}
}
