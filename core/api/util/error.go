package util

import (
	"errors"
	"net/http"
)

// 400
type ValidationError struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// 401
type UnauthorizedError struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// 403
type ForbiddenError struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// 404
type NotFoundError struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// 500
type ServerError struct {
	Error struct {
		Message string `json:"message"`
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
	case http.StatusNotFound:
		return NotFoundError{
			Error: struct {
				Message string `json:"message"`
			}{
				Message: message,
			},
		}

	case http.StatusUnauthorized:
		return UnauthorizedError{
			Error: struct {
				Message string `json:"message"`
			}{
				Message: message,
			},
		}

	case http.StatusBadRequest:
		return ValidationError{
			Error: struct {
				Message string `json:"message"`
			}{
				Message: message,
			},
		}

	default:
		return ServerError{
			Error: struct {
				Message string `json:"message"`
			}{
				Message: message,
			},
		}
	}
}
