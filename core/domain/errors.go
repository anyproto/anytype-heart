package domain

import "errors"

var ErrFileNotFound = errors.New("file not found")

var ErrValidationFailed = errors.New("validation failed")
