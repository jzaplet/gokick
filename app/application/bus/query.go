package bus

type QueryBus struct{ *Bus }

func NewQueryBus(middlewares ...Middleware) *QueryBus {
	return &QueryBus{Bus: newBus(middlewares...)}
}
