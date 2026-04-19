package bus

type CommandBus struct{ *Bus }

func NewCommandBus(middlewares ...Middleware) *CommandBus {
	return &CommandBus{Bus: newBus(middlewares...)}
}
