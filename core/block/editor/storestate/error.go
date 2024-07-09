package storestate

import "errors"

var (
	ErrIgnore            = errors.New("ignore")
	ErrLog               = errors.New("log")
	ErrUnexpectedHandler = errors.New("unexpected handler")
)
