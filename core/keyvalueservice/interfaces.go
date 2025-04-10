package keyvalueservice

import (
	"context"

	"github.com/anyproto/any-sync/app"
)

type ObserverFunc func(key string, val Value)

type Value struct {
	Data           []byte
	TimestampMilli int
}

type Service interface {
	app.ComponentRunnable

	GetUserScopedKey(ctx context.Context, key string) ([]Value, error)
	SetUserScopedKey(ctx context.Context, key string, value []byte) error
	SubscribeForUserScopedKey(key string, subscriptionName string, observerFunc ObserverFunc) error
	UnsubscribeFromUserScopedKey(key string, subscriptionName string) error
}
