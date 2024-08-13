package conc

import (
	"os"
	"runtime/debug"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/samber/lo/parallel"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("anytype-mw-panic")

func MapErr[T, R any](input []T, f func(T) (R, error)) ([]R, error) {
	var (
		allErrors error
		errLock   sync.Mutex
	)

	res := parallel.Map(input, func(in T, _ int) R {
		out, err := f(in)
		if err != nil {
			errLock.Lock()
			if allErrors == nil {
				allErrors = err
			} else {
				allErrors = multierror.Append(allErrors, err)
			}
			errLock.Unlock()
		}
		return out
	})

	return res, allErrors
}

func Go(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				if rerr, ok := r.(error); ok {
					OnPanic(rerr)
				}
			}
		}()
		fn()
	}()
}

func OnPanic(v any) {
	stack := debug.Stack()
	os.Stderr.Write(stack)
	log.With("stack", stack).Errorf("panic recovered: %v", v)
}
