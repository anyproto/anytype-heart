package meta

type Subscriber interface {
	Subscribe(ids ...string) Subscriber
	Unsubscribe(ids ...string) Subscriber
	Callback(f func(d Meta)) Subscriber
	Close() (err error)
}
