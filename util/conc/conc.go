package conc

import (
	"sync"

	"github.com/hashicorp/go-multierror"
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
				allErrors = multierror.Append(allErrors, err)
			}
			errLock.Unlock()
		}
		return out
	})

	return res, allErrors
}
