package domain

import "errors"

var ErrValidationFailed = errors.New("validation failed")
var ErrObjectIsDeleted = errors.New("object is deleted")
var ErrObjectNotFound = errors.New("object not found")
