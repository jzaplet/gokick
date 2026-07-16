package handler

import (
	"context"
	"net/http"
	"strconv"
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
	resp        *response.Responder
	queryBus    *bus.QueryBus
	commandBus  *bus.CommandBus
	getStats    *platformqry.GetStatsHandler
	listUsers   *platformqry.ListAllUsersHandler
	getUser     *platformqry.GetUserHandler
	listTenants *platformqry.ListTenantsHandler
	updateUser  *platformcmd.UpdatePlatformUserHandler
	deleteUser  *platformcmd.DeletePlatformUserHandler
	bulkDelete  *platformcmd.BulkDeletePlatformUsersHandler
	bulkActive  *platformcmd.BulkSetPlatformUsersActiveHandler
}

func NewPlatformHandler(
	resp *response.Responder,
	queryBus *bus.QueryBus,
	commandBus *bus.CommandBus,
	getStats *platformqry.GetStatsHandler,
	listUsers *platformqry.ListAllUsersHandler,
	getUser *platformqry.GetUserHandler,
	listTenants *platformqry.ListTenantsHandler,
	updateUser *platformcmd.UpdatePlatformUserHandler,
	deleteUser *platformcmd.DeletePlatformUserHandler,
	bulkDelete *platformcmd.BulkDeletePlatformUsersHandler,
	bulkActive *platformcmd.BulkSetPlatformUsersActiveHandler,
) *PlatformHandler {
	return &PlatformHandler{
		resp:        resp,
		queryBus:    queryBus,
		commandBus:  commandBus,
		getStats:    getStats,
		listUsers:   listUsers,
		getUser:     getUser,
		listTenants: listTenants,
		updateUser:  updateUser,
		deleteUser:  deleteUser,
		bulkDelete:  bulkDelete,
		bulkActive:  bulkActive,
	}
}

//gkts:assets/app/Platform/types/PlatformStats.ts PlatformStats
type platformStatsDTO struct {
	TenantCount int `json:"tenant_count"`
	UserCount   int `json:"user_count"`
}

//gkts:assets/app/Platform/types/PlatformUser.ts PlatformUser
type platformUserDTO struct {
	ID          string    `json:"id"`
	Nickname    string    `json:"nickname"`
	Email       string    `json:"email"`
	Role        user.Role `json:"role"`
	Active      bool      `json:"active"`
	TenantID    string    `json:"tenant_id"`
	TenantName  string    `json:"tenant_name"`
	LastLoginAt *string   `json:"last_login_at"`
}

//gkts:assets/app/Platform/types/PlatformTenant.ts PlatformTenant
type platformTenantDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Plan      string `json:"plan"`
	UserCount int    `json:"user_count"`
}

//gkts:assets/app/Platform/types/PlatformUserListResponse.ts PlatformUserListResponse
type platformUserListResponse struct {
	Items []platformUserDTO `json:"items"`
	Total int               `json:"total"`
}

//gkts:assets/app/Platform/types/PlatformTenantListResponse.ts PlatformTenantListResponse
type platformTenantListResponse struct {
	Items []platformTenantDTO `json:"items"`
	Total int                 `json:"total"`
}

//gkts:assets/app/Platform/types/PlatformBulkDeleteUsersRequest.ts PlatformBulkDeleteUsersRequest noguard
type platformBulkDeleteUsersRequest struct {
	IDs         []string `json:"ids"`
	AllFiltered bool     `json:"all_filtered"`
	Tenant      string   `json:"tenant"`
	Nickname    string   `json:"nickname"`
	Email       string   `json:"email"`
	Role        string   `json:"role"`
	Active      string   `json:"active"`
}

//gkts:assets/app/Platform/types/PlatformBulkActiveUsersRequest.ts PlatformBulkActiveUsersRequest noguard
type platformBulkActiveUsersRequest struct {
	IDs         []string `json:"ids"`
	AllFiltered bool     `json:"all_filtered"`
	Tenant      string   `json:"tenant"`
	Nickname    string   `json:"nickname"`
	Email       string   `json:"email"`
	Role        string   `json:"role"`
	Active      string   `json:"active"`
	SetActive   bool     `json:"set_active"`
}

//gkts:assets/app/Platform/types/PlatformUserFormData.ts PlatformUserFormData noguard
type platformUserRequest struct {
	Nickname string    `json:"nickname"`
	Password string    `json:"password"`
	Email    string    `json:"email"`
	Role     user.Role `json:"role"`
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
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	h.resp.JSON(r.Context(), w, http.StatusOK, platformStatsDTO{
		TenantCount: stats.TenantCount,
		UserCount:   stats.UserCount,
	})
}

func (h *PlatformHandler) Users(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	page, _ := strconv.Atoi(qs.Get("page"))
	perPage, _ := strconv.Atoi(qs.Get("per_page"))
	q := platformqry.ListAllUsersQuery{
		Page:     page,
		PerPage:  perPage,
		SortBy:   qs.Get("sort_by"),
		SortDir:  qs.Get("sort_dir"),
		Nickname: qs.Get("nickname"),
		Email:    qs.Get("email"),
		Role:     qs.Get("role"),
		Active:   qs.Get("active"),
		Tenant:   qs.Get("tenant"),
	}

	result, err := bus.Query(
		r.Context(),
		h.queryBus,
		"PlatformListUsers",
		q,
		func(ctx context.Context) (user.PlatformListPage, error) {
			return h.listUsers.Handle(ctx, q)
		},
	)
	if err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	dtos := make([]platformUserDTO, len(result.Items))
	for i, row := range result.Items {
		dtos[i] = toPlatformUserDTO(row)
	}

	h.resp.JSON(r.Context(), w, http.StatusOK, platformUserListResponse{
		Items: dtos,
		Total: result.Total,
	})
}

// GetUser returns one user in ANY tenant (GET /platform/users/{id}) — the
// cross-tenant read-one the platform edit view fetches instead of pulling the
// whole cross-tenant list and .find()ing it. A missing id surfaces as the query
// handler's 400 (Field "id"), so row is non-nil here.
func (h *PlatformHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	q := platformqry.GetUserQuery{ID: r.PathValue("id")}

	row, err := bus.Query(
		r.Context(),
		h.queryBus,
		"PlatformGetUser",
		q,
		func(ctx context.Context) (*user.PlatformRow, error) {
			return h.getUser.Handle(ctx, q)
		},
	)
	if err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	h.resp.JSON(r.Context(), w, http.StatusOK, toPlatformUserDTO(*row))
}

func (h *PlatformHandler) Tenants(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	page, _ := strconv.Atoi(qs.Get("page"))
	perPage, _ := strconv.Atoi(qs.Get("per_page"))
	q := platformqry.ListTenantsQuery{
		Page:    page,
		PerPage: perPage,
		SortBy:  qs.Get("sort_by"),
		SortDir: qs.Get("sort_dir"),
		Name:    qs.Get("name"),
		Plan:    qs.Get("plan"),
	}

	result, err := bus.Query(
		r.Context(),
		h.queryBus,
		"PlatformListTenants",
		q,
		func(ctx context.Context) (tenant.ListPage, error) {
			return h.listTenants.Handle(ctx, q)
		},
	)
	if err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	dtos := make([]platformTenantDTO, len(result.Items))
	for i, t := range result.Items {
		dtos[i] = platformTenantDTO{
			ID:        t.ID,
			Name:      t.Name,
			Plan:      t.Plan,
			UserCount: t.UserCount,
		}
	}

	h.resp.JSON(r.Context(), w, http.StatusOK, platformTenantListResponse{
		Items: dtos,
		Total: result.Total,
	})
}

func (h *PlatformHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	var body platformUserRequest
	if err := request.DecodeJSON(w, r, &body); err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	cmd := platformcmd.UpdatePlatformUserCommand{
		ID:       r.PathValue("id"),
		Nickname: body.Nickname,
		Password: body.Password,
		Email:    body.Email,
		Role:     string(body.Role),
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
		h.resp.HandleError(r.Context(), w, err)

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
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// BulkDeleteUsers removes the platform grid's selection (cross-tenant).
func (h *PlatformHandler) BulkDeleteUsers(w http.ResponseWriter, r *http.Request) {
	var body platformBulkDeleteUsersRequest
	if err := request.DecodeJSON(w, r, &body); err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	cmd := platformcmd.BulkDeletePlatformUsersCommand{
		IDs:         body.IDs,
		AllFiltered: body.AllFiltered,
		Tenant:      body.Tenant,
		Nickname:    body.Nickname,
		Email:       body.Email,
		Role:        string(body.Role),
		Active:      body.Active,
	}

	affected, err := bus.Dispatch(
		r.Context(),
		h.commandBus,
		"BulkDeletePlatformUsers",
		cmd,
		func(ctx context.Context) (int64, error) {
			return h.bulkDelete.Handle(ctx, cmd)
		},
	)
	if err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	h.resp.JSON(r.Context(), w, http.StatusOK, bulkResultDTO{Affected: affected})
}

// BulkActiveUsers flips the active flag for the platform grid's selection.
func (h *PlatformHandler) BulkActiveUsers(w http.ResponseWriter, r *http.Request) {
	var body platformBulkActiveUsersRequest
	if err := request.DecodeJSON(w, r, &body); err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	cmd := platformcmd.BulkSetPlatformUsersActiveCommand{
		IDs:         body.IDs,
		AllFiltered: body.AllFiltered,
		Tenant:      body.Tenant,
		Nickname:    body.Nickname,
		Email:       body.Email,
		Role:        string(body.Role),
		Active:      body.Active,
		SetActive:   body.SetActive,
	}

	affected, err := bus.Dispatch(
		r.Context(),
		h.commandBus,
		"BulkSetPlatformUsersActive",
		cmd,
		func(ctx context.Context) (int64, error) {
			return h.bulkActive.Handle(ctx, cmd)
		},
	)
	if err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	h.resp.JSON(r.Context(), w, http.StatusOK, bulkResultDTO{Affected: affected})
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
		Role:        user.Role(row.Role),
		Active:      row.Active,
		TenantID:    row.TenantID,
		TenantName:  row.TenantName,
		LastLoginAt: lastLogin,
	}
}
