package domain

import "errors"

var ErrFileNotFound = errors.New("file not found")

// UnwrapCodeFromError returns typed error code and original error
// If error is nil, returns 0 as convenient NULL error code
// If error is not wrapped with code, returns 1 as convenient UNKNOWN error code
func UnwrapCodeFromError[T ~int32](err error) (T, error) {
	if err == nil {
		// Null error
		return 0, nil
	}
	if coded, ok := err.(ErrorWithCode[T]); ok {
		return coded.code, coded.err
	} else {
		// Unknown error
		return 1, err
	}
}

// ErrorWithCode is a wrapper for error with typed error code
type ErrorWithCode[T ~int32] struct {
	err  error
	code T
}

func (e ErrorWithCode[T]) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

// WrapErrorWithCode wraps error with typed error code
func WrapErrorWithCode[T ~int32](err error, code T) error {
	return ErrorWithCode[T]{err, code}
}
