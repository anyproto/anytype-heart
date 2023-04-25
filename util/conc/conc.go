package conc

import (
	"errors"
	"sync"

	"github.com/samber/lo/parallel"
)

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
				allErrors = errors.Join(allErrors, err)
			}
			errLock.Unlock()
		}
		return out
	})

	return res, allErrors
}
