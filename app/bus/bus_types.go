package bus

// Wire named types pro rozlišení tří bus instancí.
type CommandBus struct{ *Bus }
type QueryBus struct{ *Bus }
type EventBus struct{ *Bus }
