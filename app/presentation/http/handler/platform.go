package handler

import (
	"context"
	"net/http"
	"time"

	"gokick/app/application/bus"
	platformcmd "gokick/app/application/platform/command"
	platformqry "gokick/app/application/platform/query"
	"gokick/app/domain/tenant"
	"gokick/app/domain/user"
	"gokick/app/presentation/http/request"
	"gokick/app/presentation/http/response"
)

// PlatformHandler serves the superadmin platform plane — cross-tenant overviews
// (gated platform:overview) plus cross-tenant user management (platform:users:*).
// Superadmin only; an admin is denied at the bus.
type PlatformHandler struct {
	queryBus    *bus.QueryBus
	commandBus  *bus.CommandBus
	getStats    *platformqry.GetStatsHandler
	listUsers   *platformqry.ListAllUsersHandler
	listTenants *platformqry.ListTenantsHandler
	updateUser  *platformcmd.UpdatePlatformUserHandler
	deleteUser  *platformcmd.DeletePlatformUserHandler
}

func NewPlatformHandler(
	queryBus *bus.QueryBus,
	commandBus *bus.CommandBus,
	getStats *platformqry.GetStatsHandler,
	listUsers *platformqry.ListAllUsersHandler,
	listTenants *platformqry.ListTenantsHandler,
	updateUser *platformcmd.UpdatePlatformUserHandler,
	deleteUser *platformcmd.DeletePlatformUserHandler,
) *PlatformHandler {
	return &PlatformHandler{
		queryBus:    queryBus,
		commandBus:  commandBus,
		getStats:    getStats,
		listUsers:   listUsers,
		listTenants: listTenants,
		updateUser:  updateUser,
		deleteUser:  deleteUser,
	}
}

//gkts:assets/app/Platform/types/PlatformStats.ts PlatformStats
type platformStatsDTO struct {
	TenantCount int `json:"tenant_count"`
	UserCount   int `json:"user_count"`
}

//gkts:assets/app/Platform/types/PlatformUser.ts PlatformUser
type platformUserDTO struct {
	ID          string  `json:"id"`
	Nickname    string  `json:"nickname"`
	Email       string  `json:"email"`
	Role        string  `json:"role"`
	Active      bool    `json:"active"`
	TenantID    string  `json:"tenant_id"`
	TenantName  string  `json:"tenant_name"`
	LastLoginAt *string `json:"last_login_at"`
}

//gkts:assets/app/Platform/types/PlatformTenant.ts PlatformTenant
type platformTenantDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Plan      string `json:"plan"`
	UserCount int    `json:"user_count"`
}

//gkts:assets/app/Admin/types/UserFormData.ts UserFormData
type platformUserRequest struct {
	Nickname string `json:"nickname"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

func (h *PlatformHandler) Stats(w http.ResponseWriter, r *http.Request) {
	q := platformqry.GetStatsQuery{}

	stats, err := bus.Query(
		r.Context(),
		h.queryBus,
		"PlatformStats",
		q,
		func(ctx context.Context) (platformqry.PlatformStats, error) {
			return h.getStats.Handle(ctx, q)
		},
	)
	if err != nil {
		response.HandleError(w, err)

		return
	}

	response.JSON(w, http.StatusOK, platformStatsDTO{
		TenantCount: stats.TenantCount,
		UserCount:   stats.UserCount,
	})
}

func (h *PlatformHandler) Users(w http.ResponseWriter, r *http.Request) {
	q := platformqry.ListAllUsersQuery{}

	rows, err := bus.Query(
		r.Context(),
		h.queryBus,
		"PlatformListUsers",
		q,
		func(ctx context.Context) ([]user.PlatformRow, error) {
			return h.listUsers.Handle(ctx, q)
		},
	)
	if err != nil {
		response.HandleError(w, err)

		return
	}

	dtos := make([]platformUserDTO, len(rows))
	for i, row := range rows {
		dtos[i] = toPlatformUserDTO(row)
	}

	response.JSON(w, http.StatusOK, dtos)
}

func (h *PlatformHandler) Tenants(w http.ResponseWriter, r *http.Request) {
	q := platformqry.ListTenantsQuery{}

	rows, err := bus.Query(
		r.Context(),
		h.queryBus,
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

func (h *PlatformHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	var body platformUserRequest
	if err := request.DecodeJSON(w, r, &body); err != nil {
		response.HandleError(w, err)

		return
	}

	cmd := platformcmd.UpdatePlatformUserCommand{
		ID:       r.PathValue("id"),
		Nickname: body.Nickname,
		Password: body.Password,
		Email:    body.Email,
		Role:     body.Role,
	}

	err := bus.DispatchVoid(
		r.Context(),
		h.commandBus,
		"PlatformUpdateUser",
		cmd,
		func(ctx context.Context) error {
			return h.updateUser.Handle(ctx, cmd)
		},
	)
	if err != nil {
		response.HandleError(w, err)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *PlatformHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	cmd := platformcmd.DeletePlatformUserCommand{ID: r.PathValue("id")}

	err := bus.DispatchVoid(
		r.Context(),
		h.commandBus,
		"PlatformDeleteUser",
		cmd,
		func(ctx context.Context) error {
			return h.deleteUser.Handle(ctx, cmd)
		},
	)
	if err != nil {
		response.HandleError(w, err)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func toPlatformUserDTO(row user.PlatformRow) platformUserDTO {
	var lastLogin *string
	if row.LastLoginAt != nil {
		s := row.LastLoginAt.Format(time.RFC3339)
		lastLogin = &s
	}

	return platformUserDTO{
		ID:          row.ID,
		Nickname:    row.Nickname,
		Email:       row.Email,
		Role:        row.Role,
		Active:      row.Active,
		TenantID:    row.TenantID,
		TenantName:  row.TenantName,
		LastLoginAt: lastLogin,
	}
}
