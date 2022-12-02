//go:build !nomutexdeadlockdetector

package mutex

import (
	"github.com/sasha-s/go-deadlock"
)

func NewLocker() *deadlock.Mutex {
	return &deadlock.Mutex{}
}
