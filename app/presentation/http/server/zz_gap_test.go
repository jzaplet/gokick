package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dashboardqry "gokick/app/application/dashboard/query"
	usercmd "gokick/app/application/user/command"
	userqry "gokick/app/application/user/query"
	"gokick/app/domain/shared"
	"gokick/app/infrastructure/config"
	"gokick/app/internal/testfx"
	"gokick/app/presentation/http/handler"
	"gokick/app/presentation/http/middleware"
)

// TestBuildMiddlewareChain_CSRFBlocksCrossSitePOST pins the http.CrossOriginProtection
// (Go 1.25 stdlib CSRF) wiring inside buildMiddlewareChain. It asserts BOTH directions
// against an otherwise-identical state-changing POST so the result is not a tautology —
// the only variable is the Sec-Fetch-Site header, which only the CSRF middleware reads:
//
//   - POST with Sec-Fetch-Site: cross-site  -> 403, terminal handler NOT reached.
//   - POST with no such header (same-origin) -> terminal handler reached (200).
//
// CORSMiddleware cannot account for the 403: cors.go only sets headers and short-circuits
// OPTIONS with 204 — it lets a POST through. So the rejection is unambiguously CSRF.
//
// Closes roadmap-99 and, by exercising the live "-> CSRF ->" element end-to-end through
// the production chain, the CSRF half of presentation-21 / presentation-22 / overview-13.
func TestBuildMiddlewareChain_CSRFBlocksCrossSitePOST(t *testing.T) {
	t.Parallel()

	newChain := func() (http.Handler, *bool) {
		reached := false
		sentinel := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			reached = true
			w.WriteHeader(http.StatusOK)
		})
		return chainOnlyServer(false).buildMiddlewareChain(sentinel), &reached
	}

	// Cross-site state-changing request: CSRF must reject before the handler.
	t.Run("cross-site POST rejected", func(t *testing.T) {
		chain, reached := newChain()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", nil)
		req.RemoteAddr = "203.0.113.7:5555"
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf(
				"cross-site POST: got %d want 403 — CrossOriginProtection not in chain; body=%s",
				rec.Code,
				rec.Body.String(),
			)
		}
		if *reached {
			t.Fatal("terminal handler ran on a cross-site POST — CSRF did not block the request")
		}
	})

	// Identical request minus the cross-site signal: must pass through to the handler.
	// Sends no Sec-Fetch-Site and no cross-origin Origin, so the request is treated as
	// same-origin/non-browser and allowed. This is the control that makes the 403 above
	// attributable to CSRF and not to some unconditional rejection of POSTs.
	t.Run("same-origin POST allowed", func(t *testing.T) {
		chain, reached := newChain()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", nil)
		req.RemoteAddr = "203.0.113.7:5555"
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf(
				"same-origin POST: got %d want 200 — chain rejected a non-cross-site POST; body=%s",
				rec.Code,
				rec.Body.String(),
			)
		}
		if !*reached {
			t.Fatal("terminal handler did not run on a same-origin POST — chain over-blocked")
		}
	})
}

// TestServer_SPAFallbackServesIndexForUnknownPath drives a request the whole way through
// the production stack — buildMiddlewareChain wrapped around registerRoutes — and asserts
// the SPA catch-all GET /{path...} is registered: a no-dot path that matches no explicit
// route still resolves (200) and returns the fallback index body rather than 404.
//
// This is the only test in the package that combines registerRoutes()+buildMiddlewareChain()
// and fires a request that actually reaches the SPA handler, so it closes presentation-33 and
// the "SPA fallback" third of overview-58 (the routing + chain thirds are covered by the
// existing TestRegisterRoutes_* / TestBuildMiddlewareChain_* tests).
//
// Scope: this asserts ROUTING (the catch-all is wired and reachable through the full chain),
// not file-serving/MIME behavior of SPAHandler itself — that is presentation-45 in the
// handler package. routingServer's SPA handler is built over an empty fstest.MapFS, so
// NewSPAHandler falls back to its built-in default index, which is what a no-dot path returns.
func TestServer_SPAFallbackServesIndexForUnknownPath(t *testing.T) {
	t.Parallel()

	s := routingServer(t)
	stack := s.buildMiddlewareChain(s.registerRoutes())

	// No dot in the path => SPAHandler.Serve takes the fallback branch (not file-serving),
	// and the path matches no explicit route => only GET /{path...} can handle it.
	req := httptest.NewRequest(http.MethodGet, "/some/deep/app/route", nil)
	req.RemoteAddr = "203.0.113.7:5555"
	rec := httptest.NewRecorder()
	stack.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"GET unknown no-dot path: got %d want 200 — SPA catch-all not registered/reachable; body=%s",
			rec.Code,
			rec.Body.String(),
		)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("SPA fallback Content-Type: got %q want text/html; charset=utf-8", ct)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("SPA fallback returned an empty body — expected the index document")
	}
}

// boundServer builds a Server with the REAL admin-users + dashboard handlers (wired from a
// testfx fixture exactly as the sibling handler tests do) so a request driven through the
// production mux actually executes the handler each route is bound to. Only the fields the
// admin/dashboard routes touch are populated; the other handler fields stay nil (their routes
// are never hit here). registerRoutes wraps every protected route in AuthMiddleware(jwt), so
// the test must send a valid admin Bearer for the handler to run.
//
// The bus AuthorizeMiddleware (inside each handler's bus.Exec) reads the role straight from
// the JWT claims, so the actor user need not exist in the DB — an admin role passes every
// admin:* permission. Targets that commands mutate by id (Update/Delete) ARE seeded.
func boundServer(t *testing.T) (*Server, *testfx.Fixture) {
	t.Helper()

	fx := testfx.New(t, filepath.Join(t.TempDir(), "server_bind.db"))
	cmdBus, qryBus, _ := fx.NewBuses()
	logger := silentLogger()
	extract := middleware.NewIPExtractor(false)
	rule := middleware.RateRule{Tokens: 1000, Per: time.Minute}

	adminUsers := handler.NewAdminUsersHandler(
		cmdBus,
		qryBus,
		userqry.NewListUsersHandler(fx.Users),
		usercmd.NewCreateUserHandler(fx.Users, fx.Hasher),
		usercmd.NewUpdateUserHandler(fx.Users, fx.Hasher),
		usercmd.NewDeleteUserHandler(fx.Users),
	)
	dashboard := handler.NewDashboardHandler(
		qryBus,
		dashboardqry.NewGetUserDashboardHandler(),
		dashboardqry.NewGetAdminDashboardHandler(),
	)

	s := &Server{
		config:    &config.Config{CookieSecure: false, CORSOrigin: "*"},
		logger:    logger,
		reporter:  shared.NopReporter{},
		jwt:       fx.Jwt,
		ipExtract: extract,
		limiters: &RateLimiters{
			Login:   middleware.NewRateLimiter(rule, extract, logger),
			Refresh: middleware.NewRateLimiter(rule, extract, logger),
		},
		adminUsers: adminUsers,
		dashboard:  dashboard,
	}

	return s, fx
}

// adminBearer mints a valid access token carrying an admin role for the given user id.
func adminBearer(t *testing.T, fx *testfx.Fixture, userID string) string {
	t.Helper()
	tok, _, err := fx.Jwt.GenerateAccessToken(&shared.AuthClaims{
		UserID:   userID,
		Role:     "admin",
		Nickname: "rootadmin",
	})
	if err != nil {
		t.Fatalf("mint admin token: %v", err)
	}
	return "Bearer " + tok
}

// TestRegisterRoutes_BindsAdminAndDashboardRoutesToHandlers proves each protected route is
// bound to the CORRECT handler — not merely registered and auth-wrapped (that is the existing
// TestRegisterRoutes_ProtectedRoutesRejectInvalidBearer) nor merely that each command/query
// declares the right permission (that is the *_RequiredPermission tests in the command/query
// packages). It drives an authorized admin request through the real mux and asserts each
// route lands on the handler whose distinctive effect proves the binding:
//
//   - GET    /api/v1/admin/users        -> List   -> 200 (a Create/Update/Delete mis-wire is not 200)   presentation-39
//   - POST   /api/v1/admin/users        -> Create -> 201 (unique to Create; List=200, Delete=204)       presentation-40
//   - PUT    /api/v1/admin/users/{id}   -> Update -> 204 + target survives with the changed role        presentation-41
//   - DELETE /api/v1/admin/users/{id}   -> Delete -> 204 + target is gone                                presentation-42
//   - GET    /api/v1/dashboard/user     -> User   -> 200 + "user dashboard" body                         presentation-37
//   - GET    /api/v1/dashboard/admin    -> Admin  -> 200 + "admin dashboard" body
//
// PUT and DELETE both return 204, so status alone can't separate Update from Delete against a
// 177<->178 handler swap; the post-state assertions (survives-with-new-role vs gone) pin them
// independently. Mux only (no buildMiddlewareChain) so CSRF doesn't gate the POST/PUT/DELETE —
// CSRF is covered separately above. NOTE: an admin token passes every admin:* permission, so
// this does NOT assert each route's permission *string* (e.g. dashboard:read on the user
// dashboard); those belong to the command/query packages.
func TestRegisterRoutes_BindsAdminAndDashboardRoutesToHandlers(t *testing.T) {
	t.Parallel()

	s, fx := boundServer(t)
	mux := s.registerRoutes()
	bearer := adminBearer(t, fx, "00000000-0000-0000-0000-0000000000ad")

	do := func(method, path string, body any) *httptest.ResponseRecorder {
		var rdr *bytes.Reader
		if body != nil {
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal body: %v", err)
			}
			rdr = bytes.NewReader(raw)
		} else {
			rdr = bytes.NewReader(nil)
		}
		req := httptest.NewRequest(method, path, rdr)
		req.Header.Set("Authorization", bearer)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	// GET /api/v1/admin/users -> ListUsersHandler. List always 200s (even with zero rows);
	// every other admin-users handler would yield a different status, so 200 pins List.
	t.Run("GET admin/users -> List (200)", func(t *testing.T) {
		rec := do(http.MethodGet, "/api/v1/admin/users", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("got %d want 200 — GET /api/v1/admin/users not bound to List; body=%s",
				rec.Code, rec.Body.String())
		}
	})

	// POST /api/v1/admin/users -> CreateUserHandler. 201 is unique to Create among the
	// admin-users handlers (List=200, Update/Delete=204), so it pins the binding.
	t.Run("POST admin/users -> Create (201)", func(t *testing.T) {
		rec := do(http.MethodPost, "/api/v1/admin/users", map[string]string{
			"nickname": "freshuser",
			"password": "a-valid-password",
			"email":    "freshuser@example.com",
			"role":     "user",
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("got %d want 201 — POST /api/v1/admin/users not bound to Create; body=%s",
				rec.Code, rec.Body.String())
		}
	})

	// PUT /api/v1/admin/users/{id} -> UpdateUserHandler. Update and Delete both return 204,
	// so status can't separate them; the post-state (target still present with role flipped
	// user->admin) proves the request hit Update, not Delete.
	t.Run(
		"PUT admin/users/{id} -> Update (204, target survives with new role)",
		func(t *testing.T) {
			target := fx.SeedUser(t, "puttarget", "old-password", "user")
			rec := do(http.MethodPut, "/api/v1/admin/users/"+target.ID, map[string]string{
				"nickname": "puttarget", // unchanged nickname avoids the conflict-lookup branch
				"password": "",          // empty = unchanged
				"email":    "puttarget@example.com",
				"role":     "admin", // the observable mutation that distinguishes Update from Delete
			})
			if rec.Code != http.StatusNoContent {
				t.Fatalf(
					"got %d want 204 — PUT not bound to Update; body=%s",
					rec.Code,
					rec.Body.String(),
				)
			}
			got, err := fx.Users.FindByID(context.Background(), target.ID)
			if err != nil {
				t.Fatalf(
					"target user missing after PUT — route deleted instead of updated: %v",
					err,
				)
			}
			if got.Role != "admin" {
				t.Fatalf(
					"target role after PUT: got %q want admin — Update did not apply the role change",
					got.Role,
				)
			}
		},
	)

	// DELETE /api/v1/admin/users/{id} -> DeleteUserHandler. Same 204 as Update; the post-state
	// (target removed) proves the request hit Delete, not Update.
	t.Run("DELETE admin/users/{id} -> Delete (204, target gone)", func(t *testing.T) {
		target := fx.SeedUser(t, "deltarget", "pw-to-delete", "user")
		rec := do(http.MethodDelete, "/api/v1/admin/users/"+target.ID, nil)
		if rec.Code != http.StatusNoContent {
			t.Fatalf(
				"got %d want 204 — DELETE not bound to Delete; body=%s",
				rec.Code,
				rec.Body.String(),
			)
		}
		if _, err := fx.Users.FindByID(context.Background(), target.ID); err == nil {
			t.Fatal("target user still present after DELETE — route updated instead of deleting")
		}
	})

	// GET /api/v1/dashboard/user -> DashboardHandler.User. Both dashboard routes 200, so the
	// body's "user dashboard" message (vs "admin dashboard") pins User vs Admin.
	t.Run("GET dashboard/user -> User (200, user-dashboard body)", func(t *testing.T) {
		rec := do(http.MethodGet, "/api/v1/dashboard/user", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf(
				"got %d want 200 — dashboard/user not bound to User; body=%s",
				rec.Code,
				rec.Body.String(),
			)
		}
		if !strings.Contains(rec.Body.String(), "user dashboard") {
			t.Fatalf(
				"dashboard/user body %q missing 'user dashboard' — route bound to the wrong query",
				rec.Body.String(),
			)
		}
	})

	// GET /api/v1/dashboard/admin -> DashboardHandler.Admin.
	t.Run("GET dashboard/admin -> Admin (200, admin-dashboard body)", func(t *testing.T) {
		rec := do(http.MethodGet, "/api/v1/dashboard/admin", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf(
				"got %d want 200 — dashboard/admin not bound to Admin; body=%s",
				rec.Code,
				rec.Body.String(),
			)
		}
		if !strings.Contains(rec.Body.String(), "admin dashboard") {
			t.Fatalf(
				"dashboard/admin body %q missing 'admin dashboard' — route bound to the wrong query",
				rec.Body.String(),
			)
		}
	})
}
