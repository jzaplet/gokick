package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type traceIDKeyType struct{}

var traceIDKey = traceIDKeyType{}

func TraceMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			traceID := r.Header.Get("X-Trace-Id")
			if traceID == "" {
				traceID = uuid.New().String()
			}

			ctx := context.WithValue(r.Context(), traceIDKey, traceID)
			w.Header().Set("X-Trace-Id", traceID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func TraceIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(traceIDKey).(string)
	return id
}
