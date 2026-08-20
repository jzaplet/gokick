package middleware

import (
	"net/http"
	"strings"

	"gokick/app/domain/shared"
	"gokick/app/domain/shared/msgkey"
	"gokick/app/presentation/http/response"
)

const bearerPrefix = "Bearer "

// AuthMiddleware parses an incoming "Authorization: Bearer <token>" header.
// Behavior:
//   - no header       → request passes through without claims (public route compatible)
//   - header present, malformed or token invalid/expired → 401
//   - header valid    → claims stored in context via shared.ContextWithClaims
//
// Actual permission enforcement happens in the bus AuthorizeMiddleware.
func AuthMiddleware(
	jwt shared.TokenService,
	resp *response.Responder,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				next.ServeHTTP(w, r)

				return
			}

			if !strings.HasPrefix(header, bearerPrefix) {
				resp.HandleError(
					r.Context(), w, &shared.AuthError{Key: msgkey.AuthAuthorizationHeaderInvalid},
				)

				return
			}

			token := strings.TrimPrefix(header, bearerPrefix)

			claims, err := jwt.ValidateAccessToken(token)
			if err != nil {
				resp.HandleError(r.Context(), w, err)

				return
			}

			ctx := shared.ContextWithClaims(r.Context(), claims)
			// Persisted-preference override (resolution order:
			// header → cookie → users.lang → Accept-Language → en). The
			// global LangMiddleware already resolved the request rungs and
			// stamped ctx with whether the choice was explicit; the JWT's lang
			// claim only wins when the client did not choose explicitly.
			ctx = applyClaimsLang(ctx, claims.Lang)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
