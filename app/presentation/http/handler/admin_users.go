package handler

import (
	"context"
	"net/http"
	"strconv"

	"gokick/app/application/bus"
	usercmd "gokick/app/application/user/command"
	userqry "gokick/app/application/user/query"
	"gokick/app/domain/user"
	"gokick/app/presentation/http/request"
	"gokick/app/presentation/http/response"
)

type AdminUsersHandler struct {
	resp       *response.Responder
	commandBus *bus.CommandBus
	queryBus   *bus.QueryBus
	listUsers  *userqry.ListUsersHandler
	getUser    *userqry.GetUserHandler
	createUser *usercmd.CreateUserHandler
	updateUser *usercmd.UpdateUserHandler
	deleteUser *usercmd.DeleteUserHandler
	bulkDelete *usercmd.BulkDeleteUsersHandler
	bulkActive *usercmd.BulkSetUsersActiveHandler
}

func NewAdminUsersHandler(
	resp *response.Responder,
	commandBus *bus.CommandBus,
	queryBus *bus.QueryBus,
	listUsers *userqry.ListUsersHandler,
	getUser *userqry.GetUserHandler,
	createUser *usercmd.CreateUserHandler,
	updateUser *usercmd.UpdateUserHandler,
	deleteUser *usercmd.DeleteUserHandler,
	bulkDelete *usercmd.BulkDeleteUsersHandler,
	bulkActive *usercmd.BulkSetUsersActiveHandler,
) *AdminUsersHandler {
	return &AdminUsersHandler{
		resp:       resp,
		commandBus: commandBus,
		queryBus:   queryBus,
		listUsers:  listUsers,
		getUser:    getUser,
		createUser: createUser,
		updateUser: updateUser,
		deleteUser: deleteUser,
		bulkDelete: bulkDelete,
		bulkActive: bulkActive,
	}
}

//gkts:assets/app/Admin/types/AdminUser.ts AdminUser
type adminUserDTO struct {
	ID       string `json:"id"`
	Nickname string `json:"nickname"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Active   bool   `json:"active"`
}

//gkts:assets/app/Admin/types/UserFormData.ts UserFormData noguard
type createUserRequest struct {
	Nickname string `json:"nickname"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

//gkts:assets/app/Admin/types/UserFormData.ts UserFormData noguard
type updateUserRequest struct {
	Nickname string `json:"nickname"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

//gkts:assets/app/Admin/types/AdminUserListResponse.ts AdminUserListResponse
type adminUserListResponse struct {
	Items []adminUserDTO `json:"items"`
	Total int            `json:"total"`
}

//gkts:assets/app/Admin/types/BulkDeleteUsersRequest.ts BulkDeleteUsersRequest noguard
type bulkDeleteUsersRequest struct {
	IDs         []string `json:"ids"`
	AllFiltered bool     `json:"all_filtered"`
	Nickname    string   `json:"nickname"`
	Email       string   `json:"email"`
	Role        string   `json:"role"`
	Active      string   `json:"active"`
}

//gkts:assets/app/Admin/types/BulkActiveUsersRequest.ts BulkActiveUsersRequest noguard
type bulkActiveUsersRequest struct {
	IDs         []string `json:"ids"`
	AllFiltered bool     `json:"all_filtered"`
	Nickname    string   `json:"nickname"`
	Email       string   `json:"email"`
	Role        string   `json:"role"`
	Active      string   `json:"active"`
	SetActive   bool     `json:"set_active"`
}

// bulkResultDTO is the shape both admin and platform bulk endpoints return —
// the affected-row count, so the client can tell "N changed" from "the
// selection collapsed to rows the server spared" (a bare 204 could not). Shared
// across handlers (same package); one gkts annotation.
//
//gkts:assets/app-ui/BulkActions/BulkResult.ts BulkResult
type bulkResultDTO struct {
	Affected int64 `json:"affected"`
}

// listUsersQueryFromRequest maps the grid's query string onto the query
// object. Values arrive raw — the query handler whitelists sort and clamps
// paging, so a hand-edited deep link degrades gracefully instead of erroring.
func listUsersQueryFromRequest(r *http.Request) userqry.ListUsersQuery {
	qs := r.URL.Query()
	page, _ := strconv.Atoi(qs.Get("page"))
	perPage, _ := strconv.Atoi(qs.Get("per_page"))
	return userqry.ListUsersQuery{
		Page:     page,
		PerPage:  perPage,
		SortBy:   qs.Get("sort_by"),
		SortDir:  qs.Get("sort_dir"),
		Nickname: qs.Get("nickname"),
		Email:    qs.Get("email"),
		Role:     qs.Get("role"),
		Active:   qs.Get("active"),
	}
}

func (h *AdminUsersHandler) List(w http.ResponseWriter, r *http.Request) {
	q := listUsersQueryFromRequest(r)

	page, err := bus.Query(
		r.Context(),
		h.queryBus,
		"ListUsers",
		q,
		func(ctx context.Context) (user.ListPage, error) {
			return h.listUsers.Handle(ctx, q)
		},
	)
	if err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	dtos := make([]adminUserDTO, len(page.Items))
	for i, u := range page.Items {
		dtos[i] = toAdminUserDTO(u)
	}

	h.resp.JSON(r.Context(), w, http.StatusOK, adminUserListResponse{
		Items: dtos,
		Total: page.Total,
	})
}

// Get returns one user in the caller's tenant (GET /admin/users/{id}) — the
// read-one the edit view fetches instead of pulling the whole list and .find()ing
// client-side. A missing / cross-tenant / superadmin id surfaces as the query
// handler's 400 (Field "id"), so u is non-nil here.
func (h *AdminUsersHandler) Get(w http.ResponseWriter, r *http.Request) {
	q := userqry.GetUserQuery{ID: r.PathValue("id")}

	u, err := bus.Query(
		r.Context(),
		h.queryBus,
		"GetUser",
		q,
		func(ctx context.Context) (*user.User, error) {
			return h.getUser.Handle(ctx, q)
		},
	)
	if err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	h.resp.JSON(r.Context(), w, http.StatusOK, toAdminUserDTO(*u))
}

func (h *AdminUsersHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body createUserRequest
	if err := request.DecodeJSON(w, r, &body); err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	cmd := usercmd.CreateUserCommand{
		Nickname: body.Nickname,
		Password: body.Password,
		Email:    body.Email,
		Role:     body.Role,
	}

	err := bus.DispatchVoid(
		r.Context(),
		h.commandBus,
		"CreateUser",
		cmd,
		func(ctx context.Context) error {
			return h.createUser.Handle(ctx, cmd)
		},
	)
	if err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *AdminUsersHandler) Update(w http.ResponseWriter, r *http.Request) {
	var body updateUserRequest
	if err := request.DecodeJSON(w, r, &body); err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	cmd := usercmd.UpdateUserCommand{
		ID:       r.PathValue("id"),
		Nickname: body.Nickname,
		Password: body.Password,
		Email:    body.Email,
		Role:     body.Role,
	}

	err := bus.DispatchVoid(
		r.Context(),
		h.commandBus,
		"UpdateUser",
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

func (h *AdminUsersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	cmd := usercmd.DeleteUserCommand{ID: r.PathValue("id")}

	err := bus.DispatchVoid(
		r.Context(),
		h.commandBus,
		"DeleteUser",
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

// BulkDelete removes the grid's selection (ids, or the filter set when
// all_filtered) — POST because the selection travels in the body.
func (h *AdminUsersHandler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	var body bulkDeleteUsersRequest
	if err := request.DecodeJSON(w, r, &body); err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	cmd := usercmd.BulkDeleteUsersCommand{
		IDs:         body.IDs,
		AllFiltered: body.AllFiltered,
		Nickname:    body.Nickname,
		Email:       body.Email,
		Role:        body.Role,
		Active:      body.Active,
	}

	affected, err := bus.Dispatch(
		r.Context(),
		h.commandBus,
		"BulkDeleteUsers",
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

// BulkActive flips the active flag for the grid's selection.
func (h *AdminUsersHandler) BulkActive(w http.ResponseWriter, r *http.Request) {
	var body bulkActiveUsersRequest
	if err := request.DecodeJSON(w, r, &body); err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	cmd := usercmd.BulkSetUsersActiveCommand{
		IDs:         body.IDs,
		AllFiltered: body.AllFiltered,
		Nickname:    body.Nickname,
		Email:       body.Email,
		Role:        body.Role,
		Active:      body.Active,
		SetActive:   body.SetActive,
	}

	affected, err := bus.Dispatch(
		r.Context(),
		h.commandBus,
		"BulkSetUsersActive",
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

func toAdminUserDTO(u user.User) adminUserDTO {
	return adminUserDTO{
		ID:       u.ID,
		Nickname: u.Nickname,
		Email:    u.Email,
		Role:     u.Role,
		Active:   u.Active,
	}
}
