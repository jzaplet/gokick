package handler

import (
	"context"
	"net/http"

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

//gkts:assets/app/Admin/types/UserFormData.ts UserFormData
type createUserRequest struct {
	Nickname string `json:"nickname"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

//gkts:assets/app/Admin/types/UserFormData.ts UserFormData
type updateUserRequest struct {
	Nickname string `json:"nickname"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

func (h *AdminUsersHandler) List(w http.ResponseWriter, r *http.Request) {
	q := userqry.ListUsersQuery{}

	users, err := bus.Query(
		r.Context(),
		h.queryBus,
		"ListUsers",
		q,
		func(ctx context.Context) ([]user.User, error) {
			return h.listUsers.Handle(ctx, q)
		},
	)
	if err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	dtos := make([]adminUserDTO, len(users))
	for i, u := range users {
		dtos[i] = toAdminUserDTO(u)
	}

	h.resp.JSON(r.Context(), w, http.StatusOK, dtos)
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

func toAdminUserDTO(u user.User) adminUserDTO {
	return adminUserDTO{
		ID:       u.ID,
		Nickname: u.Nickname,
		Email:    u.Email,
		Role:     u.Role,
		Active:   u.Active,
	}
}
