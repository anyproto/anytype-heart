package storestate

import "errors"

var (
	ErrIgnore            = errors.New("ignore")
	ErrValidation        = errors.New("validation")
	ErrCritical          = errors.New("critical")
	errModifier          = errors.New("modifier")
	ErrUnexpectedHandler = errors.New("unexpected handler")
	ErrOrderNotFound     = errors.New("order not found")
)
