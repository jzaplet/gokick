package shared

import "context"

type traceIDKey struct{}

func TraceIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(traceIDKey{}).(string)
	return id
}

func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}
