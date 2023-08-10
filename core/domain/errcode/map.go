package errcode

import "errors"

type ErrToCode[T ~int32] struct {
	err  error
	code T
}

func To[T ~int32](err error, code T) ErrToCode[T] {
	return ErrToCode[T]{err, code}
}

func Map[T ~int32](err error, mappings ...ErrToCode[T]) T {
	if err == nil {
		return 0
	}
	for _, m := range mappings {
		if errors.Is(err, m.err) {
			return m.code
		}
	}
	// Unknown error
	return 1
}
