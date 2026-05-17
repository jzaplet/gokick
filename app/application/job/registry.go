package job

import (
	"context"
	"fmt"
	"sort"
)

// HandlerFunc processes one job invocation. payload is the raw JSON bytes
// persisted via Enqueue; the handler unmarshals into its own struct.
type HandlerFunc func(ctx context.Context, payload []byte) error

// HandlerRegistry maps job Kind → HandlerFunc. Populated once at DI wire-up.
// Lookup is read-only at runtime — Register must only be called during
// container construction, before the worker starts.
type HandlerRegistry struct {
	handlers map[string]HandlerFunc
}

func NewHandlerRegistry(handlers map[string]HandlerFunc) (*HandlerRegistry, error) {
	for kind := range handlers {
		if kind == "" {
			return nil, fmt.Errorf("job: empty kind in registry")
		}
	}
	dup := map[string]HandlerFunc{}
	for k, v := range handlers {
		dup[k] = v
	}
	return &HandlerRegistry{handlers: dup}, nil
}

func (r *HandlerRegistry) Lookup(kind string) (HandlerFunc, bool) {
	h, ok := r.handlers[kind]
	return h, ok
}

func (r *HandlerRegistry) Has(kind string) bool {
	_, ok := r.handlers[kind]
	return ok
}

// Kinds returns the registered kinds in stable order — useful for logs and
// admin tooling that wants to list "what can this binary process?".
func (r *HandlerRegistry) Kinds() []string {
	out := make([]string, 0, len(r.handlers))
	for k := range r.handlers {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
