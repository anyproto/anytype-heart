package notion

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/import/notion/api/client"
	"github.com/anyproto/anytype-heart/pb"
)

func Test_ValidateTokenNotValid(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"object":"error","status":401,"code":"unauthorized","message":"unauthorized"}`))
	}))

	defer s.Close()
	c := client.NewClient()
	c.BasePath = s.URL

	p := NewPingService(c)
	tv := NewTokenValidator()
	tv.ping = p

	errCode, err := tv.Validate(context.TODO(), "123123")
	assert.Equal(t, errCode, pb.RpcObjectImportNotionValidateTokenResponseError_UNAUTHORIZED)
	assert.Equal(t, nil, err)
}

func Test_ValidateTokenSuccess(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	defer s.Close()
	c := client.NewClient()
	c.BasePath = s.URL

	p := NewPingService(c)
	tv := NewTokenValidator()
	tv.ping = p

	errCode, err := tv.Validate(context.TODO(), "123123")
	assert.Equal(t, errCode, pb.RpcObjectImportNotionValidateTokenResponseError_NULL)
	assert.Equal(t, nil, err)
}

func Test_ValidateTokenInternalError(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"object":"error","status":400,"code":"invalid_json","message":"The request body could not be decoded as JSON"}`))
	}))

	defer s.Close()
	c := client.NewClient()
	c.BasePath = s.URL

	p := NewPingService(c)
	tv := NewTokenValidator()
	tv.ping = p

	errCode, err := tv.Validate(context.TODO(), "123123")
	assert.Equal(t, errCode, pb.RpcObjectImportNotionValidateTokenResponseError_INTERNAL_ERROR)
	assert.NotNil(t, err)
}

func Test_ValidateTokenNotionUnavailable(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"object":"error","status":503,"code":"service_unavailable","message":"Notion is unavailable. Try again later.}`))
	}))

	defer s.Close()
	c := client.NewClient()
	c.BasePath = s.URL

	p := NewPingService(c)
	tv := NewTokenValidator()
	tv.ping = p

	errCode, err := tv.Validate(context.TODO(), "123123")
	assert.Equal(t, errCode, pb.RpcObjectImportNotionValidateTokenResponseError_SERVICE_UNAVAILABLE)
	assert.Equal(t, nil, err)
}

func Test_ValidateTokenNotionForbidden(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"object":"error","status":403,"code":"restricted_resource","message":"	Given the bearer token used, the client doesn't have permission to perform this operation.}`))
	}))

	defer s.Close()
	c := client.NewClient()
	c.BasePath = s.URL

	p := NewPingService(c)
	tv := NewTokenValidator()
	tv.ping = p

	errCode, err := tv.Validate(context.TODO(), "123123")
	assert.Equal(t, errCode, pb.RpcObjectImportNotionValidateTokenResponseError_FORBIDDEN)
	assert.Equal(t, nil, err)
}
