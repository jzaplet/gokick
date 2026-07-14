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
	resp           *response.Responder
	commandBus     *bus.CommandBus
	queryBus       *bus.QueryBus
	getProfile     *profileqry.GetProfileHandler
	changePassword *profilecmd.ChangePasswordHandler
	registry       *shared.PermissionsRegistry
}

func NewProfileHandler(
	resp *response.Responder,
	commandBus *bus.CommandBus,
	queryBus *bus.QueryBus,
	getProfile *profileqry.GetProfileHandler,
	changePassword *profilecmd.ChangePasswordHandler,
	registry *shared.PermissionsRegistry,
) *ProfileHandler {
	return &ProfileHandler{
		resp:           resp,
		commandBus:     commandBus,
		queryBus:       queryBus,
		getProfile:     getProfile,
		changePassword: changePassword,
		registry:       registry,
	}
}

//gkts:assets/app/Profile/types/ChangePasswordFormData.ts ChangePasswordFormData
type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (h *ProfileHandler) Get(w http.ResponseWriter, r *http.Request) {
	q := profileqry.GetProfileQuery{}

	u, err := bus.Query(
		r.Context(),
		h.queryBus,
		"GetProfile",
		q,
		func(ctx context.Context) (*user.User, error) {
			return h.getProfile.Handle(ctx, q)
		},
	)
	if err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	h.resp.JSON(r.Context(), w, http.StatusOK, userDTO{
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
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	cmd := profilecmd.ChangePasswordCommand{
		OldPassword: body.OldPassword,
		NewPassword: body.NewPassword,
	}

	err := bus.DispatchVoid(
		r.Context(),
		h.commandBus,
		"ChangePassword",
		cmd,
		func(ctx context.Context) error {
			return h.changePassword.Handle(ctx, cmd)
		},
	)
	if err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}
