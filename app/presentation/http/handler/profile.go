package handler

import (
	"context"
	"net/http"

	"gokick/app/application/bus"
	profilecmd "gokick/app/application/profile/command"
	profileqry "gokick/app/application/profile/query"
	"gokick/app/domain/shared"
	"gokick/app/domain/user"
	"gokick/app/presentation/http/request"
	"gokick/app/presentation/http/response"
)

type ProfileHandler struct {
	commandBus     *bus.CommandBus
	queryBus       *bus.QueryBus
	getProfile     *profileqry.GetProfileHandler
	changePassword *profilecmd.ChangePasswordHandler
	registry       *shared.PermissionsRegistry
}

func NewProfileHandler(
	commandBus *bus.CommandBus,
	queryBus *bus.QueryBus,
	getProfile *profileqry.GetProfileHandler,
	changePassword *profilecmd.ChangePasswordHandler,
	registry *shared.PermissionsRegistry,
) *ProfileHandler {
	return &ProfileHandler{
		commandBus:     commandBus,
		queryBus:       queryBus,
		getProfile:     getProfile,
		changePassword: changePassword,
		registry:       registry,
	}
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (h *ProfileHandler) Get(w http.ResponseWriter, r *http.Request) {
	q := profileqry.GetProfileQuery{}

	u, err := bus.Exec(
		r.Context(),
		h.queryBus.Bus,
		"GetProfile",
		q,
		func(ctx context.Context) (*user.User, error) {
			return h.getProfile.Handle(ctx, q)
		},
	)
	if err != nil {
		response.HandleError(w, err)

		return
	}

	response.JSON(w, http.StatusOK, userDTO{
		ID:          u.ID,
		Nickname:    u.Nickname,
		Email:       u.Email,
		Role:        u.Role,
		Permissions: h.registry.ForRole(u.Role),
	})
}

func (h *ProfileHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var body changePasswordRequest
	if err := request.DecodeJSON(w, r, &body); err != nil {
		response.Error(w, http.StatusBadRequest, err)

		return
	}

	cmd := profilecmd.ChangePasswordCommand{
		OldPassword: body.OldPassword,
		NewPassword: body.NewPassword,
	}

	err := bus.ExecVoid(
		r.Context(),
		h.commandBus.Bus,
		"ChangePassword",
		cmd,
		func(ctx context.Context) error {
			return h.changePassword.Handle(ctx, cmd)
		},
	)
	if err != nil {
		response.HandleError(w, err)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}
