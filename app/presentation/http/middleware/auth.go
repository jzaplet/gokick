package middleware

import (
	"net/http"
	"strings"

	"gokick/app/domain/shared"
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
func AuthMiddleware(jwt shared.JwtService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				next.ServeHTTP(w, r)

				return
			}

			if !strings.HasPrefix(header, bearerPrefix) {
				response.HandleError(w, &shared.AuthError{Message: "invalid Authorization header"})

				return
			}

			token := strings.TrimPrefix(header, bearerPrefix)

			claims, err := jwt.ValidateAccessToken(token)
			if err != nil {
				response.HandleError(w, err)

				return
			}

			ctx := shared.ContextWithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
