package util

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodeToApiError(t *testing.T) {
	tests := []struct {
		name          string
		code          int
		wantType      any
		wantStatus    int
		wantErrorCode string
	}{
		{"400", http.StatusBadRequest, ValidationError{}, http.StatusBadRequest, "bad_request"},
		{"401", http.StatusUnauthorized, UnauthorizedError{}, http.StatusUnauthorized, "unauthorized"},
		{"403", http.StatusForbidden, ForbiddenError{}, http.StatusForbidden, "forbidden"},
		{"404", http.StatusNotFound, NotFoundError{}, http.StatusNotFound, "object_not_found"},
		{"410", http.StatusGone, GoneError{}, http.StatusGone, "resource_gone"},
		{"429", http.StatusTooManyRequests, RateLimitError{}, http.StatusTooManyRequests, "rate_limit_exceeded"},
		{"500", http.StatusInternalServerError, ServerError{}, http.StatusInternalServerError, "internal_server_error"},
		{"unmapped 418 falls back to 500", http.StatusTeapot, ServerError{}, http.StatusInternalServerError, "internal_server_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CodeToApiError(tt.code, "msg")
			assert.IsType(t, tt.wantType, got)
			switch err := got.(type) {
			case ValidationError:
				assert.Equal(t, tt.wantStatus, err.Status)
				assert.Equal(t, tt.wantErrorCode, err.Code)
				assert.Equal(t, "msg", err.Message)
			case UnauthorizedError:
				assert.Equal(t, tt.wantStatus, err.Status)
				assert.Equal(t, tt.wantErrorCode, err.Code)
			case ForbiddenError:
				assert.Equal(t, tt.wantStatus, err.Status)
				assert.Equal(t, tt.wantErrorCode, err.Code)
			case NotFoundError:
				assert.Equal(t, tt.wantStatus, err.Status)
				assert.Equal(t, tt.wantErrorCode, err.Code)
			case GoneError:
				assert.Equal(t, tt.wantStatus, err.Status)
				assert.Equal(t, tt.wantErrorCode, err.Code)
			case RateLimitError:
				assert.Equal(t, tt.wantStatus, err.Status)
				assert.Equal(t, tt.wantErrorCode, err.Code)
			case ServerError:
				assert.Equal(t, tt.wantStatus, err.Status)
				assert.Equal(t, tt.wantErrorCode, err.Code)
			default:
				require.Fail(t, "unexpected type")
			}
		})
	}
}
