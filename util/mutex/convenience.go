package mutex

import "sync"

func WithLock[T any](mutex sync.Locker, fun func() T) T {
	mutex.Lock()
	defer mutex.Unlock()
	return fun()
}

type Value[T any] struct {
	lock  sync.Mutex
	value T
}

func NewValue[T any](value T) *Value[T] {
	return &Value[T]{value: value}
}

func (v *Value[T]) Get() T {
	v.lock.Lock()
	defer v.lock.Unlock()
	return v.value
}

func (v *Value[T]) Set(value T) {
	v.lock.Lock()
	defer v.lock.Unlock()
	v.value = value
}
