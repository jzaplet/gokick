package handler

import (
	"context"
	"net/http"

	"gokick/app/application/bus"
	dashboardqry "gokick/app/application/dashboard/query"
	"gokick/app/presentation/http/response"
)

type DashboardHandler struct {
	resp      *response.Responder
	queryBus  *bus.QueryBus
	userDash  *dashboardqry.GetUserDashboardHandler
	adminDash *dashboardqry.GetAdminDashboardHandler
}

func NewDashboardHandler(
	resp *response.Responder,
	queryBus *bus.QueryBus,
	userDash *dashboardqry.GetUserDashboardHandler,
	adminDash *dashboardqry.GetAdminDashboardHandler,
) *DashboardHandler {
	return &DashboardHandler{
		resp:      resp,
		queryBus:  queryBus,
		userDash:  userDash,
		adminDash: adminDash,
	}
}

//gkts:assets/app/Dashboard/types/DashboardResponse.ts DashboardResponse
type dashboardDTO struct {
	Message string `json:"message"`
}

func (h *DashboardHandler) User(w http.ResponseWriter, r *http.Request) {
	q := dashboardqry.GetUserDashboardQuery{}

	result, err := bus.Query(
		r.Context(),
		h.queryBus,
		"GetUserDashboard",
		q,
		func(ctx context.Context) (dashboardqry.UserDashboard, error) {
			return h.userDash.Handle(ctx, q)
		},
	)
	if err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	h.resp.JSON(r.Context(), w, http.StatusOK, dashboardDTO{Message: result.Message})
}

func (h *DashboardHandler) Admin(w http.ResponseWriter, r *http.Request) {
	q := dashboardqry.GetAdminDashboardQuery{}

	result, err := bus.Query(
		r.Context(),
		h.queryBus,
		"GetAdminDashboard",
		q,
		func(ctx context.Context) (dashboardqry.AdminDashboard, error) {
			return h.adminDash.Handle(ctx, q)
		},
	)
	if err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	h.resp.JSON(r.Context(), w, http.StatusOK, dashboardDTO{Message: result.Message})
}
