package adapter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
)

func TestChunkConcurrencyFor(t *testing.T) {
	// Chunk calls are independent, so concurrency is pure wall-clock — but only
	// an endpoint that serves requests in parallel benefits. The provider is
	// the user's declaration of which kind of endpoint this is.
	cases := []struct {
		name     string
		provider pb.RpcAIProvider
		want     int
	}{
		{"openai serves chunks in parallel", pb.RpcAI_OPENAI, cloudChunkConcurrency},
		{"ollama serializes on one gpu", pb.RpcAI_OLLAMA, localChunkConcurrency},
		{"lmstudio serializes too", pb.RpcAI_LMSTUDIO, localChunkConcurrency},
		{"llamacpp serializes too", pb.RpcAI_LLAMACPP, localChunkConcurrency},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// when
			got := chunkConcurrencyFor(&pb.RpcAIProviderConfig{Provider: c.provider})

			// then
			assert.Equal(t, c.want, got)
		})
	}

	t.Run("absent config is treated as local, never parallel", func(t *testing.T) {
		// given — a nil config must not fan out requests at an unknown endpoint
		// when
		got := chunkConcurrencyFor(nil)

		// then
		assert.Equal(t, localChunkConcurrency, got)
	})
}

func TestPlannerFromRequest(t *testing.T) {
	t.Run("no aiParams leaves the feature off", func(t *testing.T) {
		// when
		got := plannerFromRequest(&pb.RpcObjectImportRequest{})

		// then — converters fall back to the naive planner
		assert.Nil(t, got.planner)
		assert.False(t, got.includeSamples)
	})

	t.Run("a usable local config produces a planner", func(t *testing.T) {
		// given — ollama needs no token, and building the client makes no
		// network call, so this stays hermetic
		req := &pb.RpcObjectImportRequest{AiParams: &pb.RpcObjectImportRequestAIParams{
			Config:                &pb.RpcAIProviderConfig{Provider: pb.RpcAI_OLLAMA, Model: "gemma4:e2b"},
			IncludeContentSamples: true,
		}}

		// when
		got := plannerFromRequest(req)

		// then
		require.NotNil(t, got.planner)
		assert.True(t, got.includeSamples)
	})

	t.Run("a present but unusable config fails loudly, never silently", func(t *testing.T) {
		// given — openai without a token: the user asked for enrichment and
		// must see llmPlanFailed rather than a quietly naive import
		req := &pb.RpcObjectImportRequest{AiParams: &pb.RpcObjectImportRequestAIParams{
			Config: &pb.RpcAIProviderConfig{Provider: pb.RpcAI_OPENAI, Model: "gpt-4o-mini"},
		}}

		// when
		got := plannerFromRequest(req)

		// then
		require.NotNil(t, got.planner, "must not silently disable the feature")
		_, err := got.planner.Plan(t.Context(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ai config")
	})
}
