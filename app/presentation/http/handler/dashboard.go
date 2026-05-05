package handler

import (
	"context"
	"net/http"

	"gokick/app/application/bus"
	dashboardqry "gokick/app/application/dashboard/query"
	"gokick/app/presentation/http/response"
)

type DashboardHandler struct {
	queryBus  *bus.QueryBus
	userDash  *dashboardqry.GetUserDashboardHandler
	adminDash *dashboardqry.GetAdminDashboardHandler
}

func NewDashboardHandler(
	queryBus *bus.QueryBus,
	userDash *dashboardqry.GetUserDashboardHandler,
	adminDash *dashboardqry.GetAdminDashboardHandler,
) *DashboardHandler {
	return &DashboardHandler{
		queryBus:  queryBus,
		userDash:  userDash,
		adminDash: adminDash,
	}
}

type dashboardDTO struct {
	Message string `json:"message"`
}

func (h *DashboardHandler) User(w http.ResponseWriter, r *http.Request) {
	q := dashboardqry.GetUserDashboardQuery{}

	result, err := bus.Exec(
		r.Context(),
		h.queryBus.Bus,
		"GetUserDashboard",
		q,
		func(ctx context.Context) (dashboardqry.UserDashboard, error) {
			return h.userDash.Handle(ctx, q)
		},
	)
	if err != nil {
		response.HandleError(w, err)

		return
	}

	response.JSON(w, http.StatusOK, dashboardDTO{Message: result.Message})
}

func (h *DashboardHandler) Admin(w http.ResponseWriter, r *http.Request) {
	q := dashboardqry.GetAdminDashboardQuery{}

	result, err := bus.Exec(
		r.Context(),
		h.queryBus.Bus,
		"GetAdminDashboard",
		q,
		func(ctx context.Context) (dashboardqry.AdminDashboard, error) {
			return h.adminDash.Handle(ctx, q)
		},
	)
	if err != nil {
		response.HandleError(w, err)

		return
	}

	response.JSON(w, http.StatusOK, dashboardDTO{Message: result.Message})
}
