package util

import (
	"errors"
	"net/http"
)

// ValidationError is a struct for 400 errors
type ValidationError struct {
	Error struct {
		Message string `json:"message" example:"Bad request"`
	} `json:"error"`
}

// UnauthorizedError is a struct for 401 errors
type UnauthorizedError struct {
	Error struct {
		Message string `json:"message" example:"Unauthorized"`
	} `json:"error"`
}

// ForbiddenError is a struct for 403 errors
type ForbiddenError struct {
	Error struct {
		Message string `json:"message" example:"Forbidden"`
	} `json:"error"`
}

// NotFoundError is a struct for 404 errors
type NotFoundError struct {
	Error struct {
		Message string `json:"message" example:"Resource not found"`
	} `json:"error"`
}

// GoneError is a struct for 410 errors
type GoneError struct {
	Error struct {
		Message string `json:"message" example:"Resource is gone"`
	} `json:"error"`
}

// RateLimitError is a struct for 423 errors
type RateLimitError struct {
	Error struct {
		Message string `json:"message" example:"Rate limit exceeded"`
	} `json:"error"`
}

// ServerError is a struct for 500 errors
type ServerError struct {
	Error struct {
		Message string `json:"message" example:"Internal server error"`
	} `json:"error"`
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

// CodeToAPIError returns an instance of the correct struct
// for the given HTTP code, embedding the supplied message.
func CodeToAPIError(code int, message string) any {
	switch code {

	case http.StatusBadRequest:
		return ValidationError{
			Error: struct {
				Message string `json:"message" example:"Bad request"`
			}{
				Message: message,
			},
		}

	case http.StatusUnauthorized:
		return UnauthorizedError{
			Error: struct {
				Message string `json:"message" example:"Unauthorized"`
			}{
				Message: message,
			},
		}

	case http.StatusForbidden:
		return ForbiddenError{
			Error: struct {
				Message string `json:"message" example:"Forbidden"`
			}{
				Message: message,
			},
		}

	case http.StatusNotFound:
		return NotFoundError{
			Error: struct {
				Message string `json:"message" example:"Resource not found"`
			}{
				Message: message,
			},
		}

	case http.StatusTooManyRequests:
		return RateLimitError{
			Error: struct {
				Message string `json:"message" example:"Rate limit exceeded"`
			}{
				Message: message,
			},
		}

	default:
		return ServerError{
			Error: struct {
				Message string `json:"message" example:"Internal server error"`
			}{
				Message: message,
			},
		}
	}
}
