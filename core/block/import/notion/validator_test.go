package notion

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/client"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/ping"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func Test_ValidateTokenNotValid(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"object":"error","status":401,"code":"unauthorized","message":"unauthorized"}`))
	}))

	defer s.Close()
	c := client.NewClient()
	c.BasePath = s.URL

	p := ping.New(c)
	tv := NewTokenValidator()
	tv.ping = p

	err := tv.Validate(context.TODO(), "123123")
	assert.Equal(t, err, pb.RpcObjectImportNotionTokenValidateResponseError_UNAUTHORIZED)
}

func Test_ValidateTokenSuccess(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	defer s.Close()
	c := client.NewClient()
	c.BasePath = s.URL

	p := ping.New(c)
	tv := NewTokenValidator()
	tv.ping = p

	err := tv.Validate(context.TODO(), "123123")
	assert.Equal(t, err, pb.RpcObjectImportNotionTokenValidateResponseError_NULL)
}

func Test_ValidateTokenInternalError(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"object":"error","status":400,"code":"invalid_json","message":"The request body could not be decoded as JSON"}`))
	}))

	defer s.Close()
	c := client.NewClient()
	c.BasePath = s.URL

	p := ping.New(c)
	tv := NewTokenValidator()
	tv.ping = p

	err := tv.Validate(context.TODO(), "123123")
	assert.Equal(t, err, pb.RpcObjectImportNotionTokenValidateResponseError_INTERNAL_ERROR)
}
