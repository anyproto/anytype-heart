package session

type NewSessionNotifier interface {
	Notify(ctx Context)
}
