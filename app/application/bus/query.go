package bus

// QueryBus is the read-side bus. Its inner *Bus is unexported so a read runs only
// through bus.Query, which takes *QueryBus — the counterpart to Dispatch's
// compile-checked pairing (see CommandBus).
type QueryBus struct{ inner *Bus }

func NewQueryBus(middlewares ...Middleware) *QueryBus {
	return &QueryBus{inner: newBus(middlewares...)}
}
