package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	authcmd "gokick/app/application/auth/command"
	"gokick/app/application/bus"
	"gokick/app/domain/shared"
	"gokick/app/presentation/http/request"
	"gokick/app/presentation/http/response"
)

const refreshCookieName = "refresh_token"

// Path under which the refresh cookie is valid — limits exposure to just the auth endpoints.
const refreshCookiePath = "/api/v1/auth"

// sessionHintCookieName is a NON-HttpOnly, no-secret flag ("1") set alongside
// the refresh cookie with the same lifetime. The SPA can't read the HttpOnly
// refresh cookie, so on bootstrap it would otherwise always POST /auth/refresh —
// a guaranteed 401 (and a wasted request) for every guest. With this readable
// hint the SPA only attempts the restore when a session plausibly exists. The
// server owns it (same Expires as the refresh cookie) so it never drifts into a
// false negative that would log a real session out.
const sessionHintCookieName = "gk_session"

type AuthHandler struct {
	cookieSecure bool
	commandBus   *bus.CommandBus
	login        *authcmd.LoginHandler
	refreshToken *authcmd.RefreshTokenHandler
	logout       *authcmd.LogoutHandler
	registry     *shared.PermissionsRegistry
}

// CookieSecure is a named-type flag injected by Wire so AuthHandler does not
// need to depend on the whole *config.Config.
type CookieSecure bool

func NewAuthHandler(
	cookieSecure CookieSecure,
	commandBus *bus.CommandBus,
	login *authcmd.LoginHandler,
	refreshToken *authcmd.RefreshTokenHandler,
	logout *authcmd.LogoutHandler,
	registry *shared.PermissionsRegistry,
) *AuthHandler {
	return &AuthHandler{
		cookieSecure: bool(cookieSecure),
		commandBus:   commandBus,
		login:        login,
		refreshToken: refreshToken,
		logout:       logout,
		registry:     registry,
	}
}

type loginRequest struct {
	Nickname string `json:"nickname"`
	Password string `json:"password"`
}

type userDTO struct {
	ID          string   `json:"id"`
	Nickname    string   `json:"nickname"`
	Email       string   `json:"email"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
}

type loginResponse struct {
	AccessToken      string  `json:"access_token"`
	AccessExpiration int     `json:"access_expiration"`
	User             userDTO `json:"user"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var body loginRequest
	if err := request.DecodeJSON(w, r, &body); err != nil {
		response.Error(w, http.StatusBadRequest, err)

		return
	}

	cmd := authcmd.LoginCommand{Nickname: body.Nickname, Password: body.Password}

	result, err := bus.Exec(
		r.Context(),
		h.commandBus.Bus,
		"Login",
		cmd,
		func(ctx context.Context) (authcmd.LoginResult, error) {
			return h.login.Handle(ctx, cmd)
		},
	)
	if err != nil {
		response.HandleError(w, err)

		return
	}

	h.writeAuthResponse(w, result)
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil {
		response.HandleError(w, &shared.AuthError{Message: "missing refresh token"})

		return
	}

	cmd := authcmd.RefreshTokenCommand{RawToken: cookie.Value}

	result, err := bus.Exec(
		r.Context(),
		h.commandBus.Bus,
		"RefreshToken",
		cmd,
		func(ctx context.Context) (authcmd.LoginResult, error) {
			return h.refreshToken.Handle(ctx, cmd)
		},
	)
	if err != nil {
		// Only drop the session cookies when the refresh token itself is
		// invalid/revoked/expired (an auth-class failure → 401). A transient
		// error (DB blip → 5xx) must NOT log the user out: keep the cookie so
		// the next attempt can still succeed instead of forcing a re-login from
		// a momentary backend hiccup.
		var authErr *shared.AuthError
		if errors.As(err, &authErr) {
			h.clearRefreshCookie(w)
		}
		response.HandleError(w, err)

		return
	}

	h.writeAuthResponse(w, result)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	cmd := authcmd.LogoutCommand{}

	err := bus.ExecVoid(
		r.Context(),
		h.commandBus.Bus,
		"Logout",
		cmd,
		func(ctx context.Context) error {
			return h.logout.Handle(ctx, cmd)
		},
	)
	if err != nil {
		response.HandleError(w, err)

		return
	}

	h.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) writeAuthResponse(w http.ResponseWriter, result authcmd.LoginResult) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    result.RefreshToken,
		Path:     refreshCookiePath,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteStrictMode,
		Expires:  result.RefreshExpiresAt,
	})
	// Readable session hint at Path=/ so the SPA bootstrap can see it (NOT
	// HttpOnly, carries no secret). Same Expires as the refresh cookie above.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionHintCookieName,
		Value:    "1",
		Path:     "/",
		HttpOnly: false,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteStrictMode,
		Expires:  result.RefreshExpiresAt,
	})

	response.JSON(w, http.StatusOK, loginResponse{
		AccessToken:      result.AccessToken,
		AccessExpiration: int(result.AccessExpiresIn.Seconds()),
		User: userDTO{
			ID:          result.User.ID,
			Nickname:    result.User.Nickname,
			Email:       result.User.Email,
			Role:        result.User.Role,
			Permissions: h.registry.ForRole(result.User.Role),
		},
	})
}

func (h *AuthHandler) clearRefreshCookie(w http.ResponseWriter) {
	// MaxAge=-1 is the modern signal to delete the cookie; Expires in the
	// past is the legacy fallback for browsers (and proxies) that ignore
	// MaxAge. Belt + suspenders — costs nothing and forecloses one weird
	// "cookie still present after logout" failure mode.
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     refreshCookiePath,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
	// Clear the session hint in lock-step (must match its Path=/ to delete it).
	http.SetCookie(w, &http.Cookie{
		Name:     sessionHintCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: false,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}
