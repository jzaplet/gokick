package handler

import (
	"context"
	"net/http"
	"strconv"

	"gokick/app/application/bus"
	platformcmd "gokick/app/application/platform/command"
	platformqry "gokick/app/application/platform/query"
	"gokick/app/domain/shared"
	"gokick/app/domain/tenant"
	"gokick/app/presentation/http/request"
)

// The tenants half of the superadmin platform plane: the cross-tenant overview
// grid plus tenant create/delete. Split from platform.go (users) purely by size —
// both halves hang off the same PlatformHandler, declared there.

// IsDefault is computed here rather than mirrored as a constant on the frontend:
// the default tenant is a backend fact (shared.DefaultTenantID, created by
// migration) and the grid needs it only to explain why the delete button is off.
// A hardcoded nil-UUID in TypeScript would be the same fact stated twice, free to
// drift from the Go one — and it is the delete command, not the grid, that
// actually enforces this.
//
//gkts:assets/app/Platform/types/PlatformTenant.ts PlatformTenant
type platformTenantDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Plan      string `json:"plan"`
	UserCount int    `json:"user_count"`
	IsDefault bool   `json:"is_default"`
}

//gkts:assets/app/Platform/types/PlatformTenantListResponse.ts PlatformTenantListResponse
type platformTenantListResponse struct {
	Items []platformTenantDTO `json:"items"`
	Total int                 `json:"total"`
}

//gkts:assets/app/Platform/types/PlatformTenantFormData.ts PlatformTenantFormData noguard
type platformTenantRequest struct {
	Name string `json:"name"`
}

//gkts:assets/app/Platform/types/PlatformBulkDeleteTenantsRequest.ts PlatformBulkDeleteTenantsRequest noguard
type platformBulkDeleteTenantsRequest struct {
	IDs         []string `json:"ids"`
	AllFiltered bool     `json:"all_filtered"`
	Name        string   `json:"name"`
	Plan        string   `json:"plan"`
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
			IsDefault: t.ID == shared.DefaultTenantID,
		}
	}

	h.resp.JSON(r.Context(), w, http.StatusOK, platformTenantListResponse{
		Items: dtos,
		Total: result.Total,
	})
}

// CreateTenant adds a tenant. Dispatches the same command the CLI's create-tenant
// uses; here the bus's Authorize gate is what confines it to a superadmin.
func (h *PlatformHandler) CreateTenant(w http.ResponseWriter, r *http.Request) {
	var body platformTenantRequest
	if err := request.DecodeJSON(w, r, &body); err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	cmd := platformcmd.CreateTenantCommand{Name: body.Name}

	err := bus.DispatchVoid(
		r.Context(),
		h.commandBus,
		"PlatformCreateTenant",
		cmd,
		func(ctx context.Context) error {
			// The handler returns the created tenant; the response deliberately
			// carries no body (201, admin-create parity), so it is discarded.
			_, err := h.createTenant.Handle(ctx, cmd)

			return err
		},
	)
	if err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	w.WriteHeader(http.StatusCreated)
}

// DeleteTenant removes one tenant. Refused with a 400 when it still owns users.
func (h *PlatformHandler) DeleteTenant(w http.ResponseWriter, r *http.Request) {
	cmd := platformcmd.DeleteTenantCommand{ID: r.PathValue("id")}

	err := bus.DispatchVoid(
		r.Context(),
		h.commandBus,
		"PlatformDeleteTenant",
		cmd,
		func(ctx context.Context) error {
			return h.deleteTenant.Handle(ctx, cmd)
		},
	)
	if err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// BulkDeleteTenants removes the empty tenants in the grid's selection. Affected
// counts what actually went — tenants that still have users are skipped, so a
// partial result is the normal case, not an error.
func (h *PlatformHandler) BulkDeleteTenants(w http.ResponseWriter, r *http.Request) {
	var body platformBulkDeleteTenantsRequest
	if err := request.DecodeJSON(w, r, &body); err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	cmd := platformcmd.BulkDeleteTenantsCommand{
		IDs:         body.IDs,
		AllFiltered: body.AllFiltered,
		Name:        body.Name,
		Plan:        body.Plan,
	}

	affected, err := bus.Dispatch(
		r.Context(),
		h.commandBus,
		"BulkDeletePlatformTenants",
		cmd,
		func(ctx context.Context) (int64, error) {
			return h.bulkDeleteTenant.Handle(ctx, cmd)
		},
	)
	if err != nil {
		h.resp.HandleError(r.Context(), w, err)

		return
	}

	h.resp.JSON(r.Context(), w, http.StatusOK, bulkResultDTO{Affected: affected})
}
