package storestate

import "errors"

var (
	ErrIgnore            = errors.New("ignore")
	ErrValidation        = errors.New("validation")
	ErrCritical          = errors.New("critical")
	ErrUnexpectedHandler = errors.New("unexpected handler")
	ErrOrderNotFound     = errors.New("order not found")
)
