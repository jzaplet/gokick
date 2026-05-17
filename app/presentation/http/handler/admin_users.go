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
	commandBus *bus.CommandBus
	queryBus   *bus.QueryBus
	listUsers  *userqry.ListUsersHandler
	createUser *usercmd.CreateUserHandler
	updateUser *usercmd.UpdateUserHandler
	deleteUser *usercmd.DeleteUserHandler
}

func NewAdminUsersHandler(
	commandBus *bus.CommandBus,
	queryBus *bus.QueryBus,
	listUsers *userqry.ListUsersHandler,
	createUser *usercmd.CreateUserHandler,
	updateUser *usercmd.UpdateUserHandler,
	deleteUser *usercmd.DeleteUserHandler,
) *AdminUsersHandler {
	return &AdminUsersHandler{
		commandBus: commandBus,
		queryBus:   queryBus,
		listUsers:  listUsers,
		createUser: createUser,
		updateUser: updateUser,
		deleteUser: deleteUser,
	}
}

type adminUserDTO struct {
	ID       string `json:"id"`
	Nickname string `json:"nickname"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Active   bool   `json:"active"`
}

type createUserRequest struct {
	Nickname string `json:"nickname"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

type updateUserRequest struct {
	Nickname string `json:"nickname"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

func (h *AdminUsersHandler) List(w http.ResponseWriter, r *http.Request) {
	q := userqry.ListUsersQuery{}

	users, err := bus.Exec(
		r.Context(),
		h.queryBus.Bus,
		"ListUsers",
		q,
		func(ctx context.Context) ([]user.User, error) {
			return h.listUsers.Handle(ctx, q)
		},
	)
	if err != nil {
		response.HandleError(w, err)

		return
	}

	dtos := make([]adminUserDTO, len(users))
	for i, u := range users {
		dtos[i] = toAdminUserDTO(u)
	}

	response.JSON(w, http.StatusOK, dtos)
}

func (h *AdminUsersHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body createUserRequest
	if err := request.DecodeJSON(w, r, &body); err != nil {
		response.Error(w, http.StatusBadRequest, err)

		return
	}

	cmd := usercmd.CreateUserCommand{
		Nickname: body.Nickname,
		Password: body.Password,
		Email:    body.Email,
		Role:     body.Role,
	}

	err := bus.ExecVoid(
		r.Context(),
		h.commandBus.Bus,
		"CreateUser",
		cmd,
		func(ctx context.Context) error {
			return h.createUser.Handle(ctx, cmd)
		},
	)
	if err != nil {
		response.HandleError(w, err)

		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *AdminUsersHandler) Update(w http.ResponseWriter, r *http.Request) {
	var body updateUserRequest
	if err := request.DecodeJSON(w, r, &body); err != nil {
		response.Error(w, http.StatusBadRequest, err)

		return
	}

	cmd := usercmd.UpdateUserCommand{
		ID:       r.PathValue("id"),
		Nickname: body.Nickname,
		Password: body.Password,
		Email:    body.Email,
		Role:     body.Role,
	}

	err := bus.ExecVoid(
		r.Context(),
		h.commandBus.Bus,
		"UpdateUser",
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

func (h *AdminUsersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	cmd := usercmd.DeleteUserCommand{ID: r.PathValue("id")}

	err := bus.ExecVoid(
		r.Context(),
		h.commandBus.Bus,
		"DeleteUser",
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

func toAdminUserDTO(u user.User) adminUserDTO {
	return adminUserDTO{
		ID:       u.ID,
		Nickname: u.Nickname,
		Email:    u.Email,
		Role:     u.Role,
		Active:   u.Active,
	}
}
