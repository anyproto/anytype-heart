package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
)

type aiFixture struct {
	*Middleware
}

func newAIFixture(t *testing.T) *aiFixture {
	return &aiFixture{Middleware: &Middleware{}}
}

func TestAIListModels(t *testing.T) {
	t.Run("healthy list response is returned unfiltered for a non-openai provider", func(t *testing.T) {
		// given
		fx := newAIFixture(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/v1/models", r.URL.Path)
			_, _ = w.Write([]byte(`{"object":"list","data":[
				{"id":"qwen3:8b","object":"model","created":1,"owned_by":"library"},
				{"id":"gemma3:e4b","object":"model","created":1,"owned_by":"library"}
			]}`))
		}))
		t.Cleanup(srv.Close)
		req := &pb.RpcAIListModelsRequest{Config: &pb.RpcAIProviderConfig{
			Provider: pb.RpcAI_OLLAMA,
			Endpoint: srv.URL + "/v1",
		}}
		want := []*pb.RpcAIListModelsModel{
			{Id: "qwen3:8b", OwnedBy: "library"},
			{Id: "gemma3:e4b", OwnedBy: "library"},
		}

		// when
		got := fx.AIListModels(context.Background(), req)

		// then
		require.Equal(t, pb.RpcAIListModelsResponseError_NULL, got.Error.Code)
		assert.Equal(t, want, got.Models)
	})

	t.Run("openai catalog is filtered to chat completion models", func(t *testing.T) {
		// given
		fx := newAIFixture(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"object":"list","data":[
				{"id":"gpt-4o","object":"model","created":1,"owned_by":"openai"},
				{"id":"text-embedding-3-small","object":"model","created":1,"owned_by":"openai"},
				{"id":"whisper-1","object":"model","created":1,"owned_by":"openai"}
			]}`))
		}))
		t.Cleanup(srv.Close)
		req := &pb.RpcAIListModelsRequest{Config: &pb.RpcAIProviderConfig{
			Provider: pb.RpcAI_OPENAI,
			Endpoint: srv.URL + "/v1",
			Token:    "sk-test",
		}}
		want := []*pb.RpcAIListModelsModel{{Id: "gpt-4o", OwnedBy: "openai"}}

		// when
		got := fx.AIListModels(context.Background(), req)

		// then
		require.Equal(t, pb.RpcAIListModelsResponseError_NULL, got.Error.Code)
		assert.Equal(t, want, got.Models)
	})

	t.Run("401 maps to AUTH_REQUIRED", func(t *testing.T) {
		// given
		fx := newAIFixture(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
		}))
		t.Cleanup(srv.Close)
		req := &pb.RpcAIListModelsRequest{Config: &pb.RpcAIProviderConfig{
			Provider: pb.RpcAI_OPENAI,
			Endpoint: srv.URL + "/v1",
			Token:    "sk-bad",
		}}

		// when
		got := fx.AIListModels(context.Background(), req)

		// then
		assert.Equal(t, pb.RpcAIListModelsResponseError_AUTH_REQUIRED, got.Error.Code)
		assert.Empty(t, got.Models)
	})

	t.Run("unreachable endpoint maps to ENDPOINT_NOT_REACHABLE", func(t *testing.T) {
		// given
		fx := newAIFixture(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		url := srv.URL
		srv.Close()
		req := &pb.RpcAIListModelsRequest{Config: &pb.RpcAIProviderConfig{
			Provider: pb.RpcAI_OLLAMA,
			Endpoint: url + "/v1",
		}}

		// when
		got := fx.AIListModels(context.Background(), req)

		// then
		assert.Equal(t, pb.RpcAIListModelsResponseError_ENDPOINT_NOT_REACHABLE, got.Error.Code)
	})

	t.Run("unusable config maps to BAD_INPUT", func(t *testing.T) {
		// given — openai with no token given at all
		fx := newAIFixture(t)
		req := &pb.RpcAIListModelsRequest{Config: &pb.RpcAIProviderConfig{Provider: pb.RpcAI_OPENAI}}

		// when
		got := fx.AIListModels(context.Background(), req)

		// then
		assert.Equal(t, pb.RpcAIListModelsResponseError_BAD_INPUT, got.Error.Code)
	})
}
