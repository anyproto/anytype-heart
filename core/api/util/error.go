package util

import (
	"errors"
	"fmt"
	"net/http"
)

var (
	ErrBad = errors.New("bad input")
)

// ValidationError is a struct for 400 errors
type ValidationError struct {
	Object  string `json:"object" example:"error"`
	Status  int    `json:"status" example:"400"`
	Code    string `json:"code" example:"bad_request"`
	Message string `json:"message" example:"Bad request"`
}

// UnauthorizedError is a struct for 401 errors
type UnauthorizedError struct {
	Object  string `json:"object" example:"error"`
	Status  int    `json:"status" example:"401"`
	Code    string `json:"code" example:"unauthorized"`
	Message string `json:"message" example:"Unauthorized"`
}

// ForbiddenError is a struct for 403 errors
type ForbiddenError struct {
	Object  string `json:"object" example:"error"`
	Status  int    `json:"status" example:"403"`
	Code    string `json:"code" example:"forbidden"`
	Message string `json:"message" example:"Forbidden"`
}

// NotFoundError is a struct for 404 errors
type NotFoundError struct {
	Object  string `json:"object" example:"error"`
	Status  int    `json:"status" example:"404"`
	Code    string `json:"code" example:"object_not_found"`
	Message string `json:"message" example:"Resource not found"`
}

// GoneError is a struct for 410 errors
type GoneError struct {
	Object  string `json:"object" example:"error"`
	Status  int    `json:"status" example:"410"`
	Code    string `json:"code" example:"resource_gone"`
	Message string `json:"message" example:"Resource is gone"`
}

// RateLimitError is a struct for 423 errors
type RateLimitError struct {
	Object  string `json:"object" example:"error"`
	Status  int    `json:"status" example:"429"`
	Code    string `json:"code" example:"rate_limit_exceeded"`
	Message string `json:"message" example:"Rate limit exceeded"`
}

// ServerError is a struct for 500 errors
type ServerError struct {
	Object  string `json:"object" example:"error"`
	Status  int    `json:"status" example:"500"`
	Code    string `json:"code" example:"internal_server_error"`
	Message string `json:"message" example:"Internal server error"`
}

type errCodeMapping struct {
	target error
	code   int
}

// ErrToCode just returns a mapping to pair a target error with a code
func ErrToCode(target error, code int) errCodeMapping {
	return errCodeMapping{
		target: target,
		code:   code,
	}
}

// ErrBadInput is a sentinel error for bad input
func ErrBadInput(msg string) error {
	return fmt.Errorf("%w: %s", ErrBad, msg)
}

// MapErrorCode checks if err matches any “target” in the mappings,
// returning the first matching code. If none match, returns 500.
func MapErrorCode(err error, mappings ...errCodeMapping) int {
	if err == nil {
		return http.StatusOK
	}
	for _, m := range mappings {
		if errors.Is(err, m.target) {
			return m.code
		}
	}

	return http.StatusInternalServerError
}

// CodeToApiError returns an instance of the correct struct
// for the given HTTP code, embedding the supplied message.
func CodeToApiError(code int, message string) any {
	switch code {

	case http.StatusBadRequest:
		return ValidationError{
			Object:  "error",
			Status:  http.StatusBadRequest,
			Code:    "bad_request",
			Message: message,
		}

	case http.StatusUnauthorized:
		return UnauthorizedError{
			Object:  "error",
			Status:  http.StatusUnauthorized,
			Code:    "unauthorized",
			Message: message,
		}

	case http.StatusForbidden:
		return ForbiddenError{
			Object:  "error",
			Status:  http.StatusForbidden,
			Code:    "forbidden",
			Message: message,
		}

	case http.StatusNotFound:
		return NotFoundError{
			Object:  "error",
			Status:  http.StatusNotFound,
			Code:    "object_not_found",
			Message: message,
		}

	case http.StatusTooManyRequests:
		return RateLimitError{
			Object:  "error",
			Status:  http.StatusTooManyRequests,
			Code:    "rate_limit_exceeded",
			Message: message,
		}

	default:
		return ServerError{
			Object:  "error",
			Status:  http.StatusInternalServerError,
			Code:    "internal_server_error",
			Message: message,
		}
	}
}
