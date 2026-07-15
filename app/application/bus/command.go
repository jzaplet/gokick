package bus

// CommandBus is the write-side bus. Its inner *Bus is unexported so the only way
// to run a command is bus.Dispatch / bus.DispatchVoid, which take *CommandBus —
// making the bus↔operation pairing compile-checked (a write command can no longer
// be unwrapped to a bare *Bus and run on the QueryBus, skipping Transaction/Audit).
type CommandBus struct{ inner *Bus }

func NewCommandBus(middlewares ...Middleware) *CommandBus {
	return &CommandBus{inner: newBus(middlewares...)}
}
