//go:build !mutexdeadlockdetector

package mutex

import (
	"sync"
)

func NewLocker() *sync.Mutex {
	return &sync.Mutex{}
}
