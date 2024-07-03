package session

type Notifier interface {
	Notify(ctx Context)
}
