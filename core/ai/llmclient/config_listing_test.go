package llmclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
)

func TestFromProtoForListing(t *testing.T) {
	t.Run("nil config is an error", func(t *testing.T) {
		_, err := FromProtoForListing(nil)
		require.Error(t, err)
	})

	t.Run("empty model is fine — that's the point of listing", func(t *testing.T) {
		cfg, err := FromProtoForListing(&pb.RpcAIProviderConfig{Provider: pb.RpcAI_OLLAMA})
		require.NoError(t, err)
		assert.Equal(t, "http://localhost:11434/v1", cfg.Endpoint)
		assert.Empty(t, cfg.Model)
	})

	t.Run("provider default endpoints", func(t *testing.T) {
		for provider, want := range map[pb.RpcAIProvider]string{
			pb.RpcAI_OLLAMA:   "http://localhost:11434/v1",
			pb.RpcAI_LMSTUDIO: "http://localhost:1234/v1",
			pb.RpcAI_LLAMACPP: "http://localhost:8080/v1",
		} {
			cfg, err := FromProtoForListing(&pb.RpcAIProviderConfig{Provider: provider})
			require.NoError(t, err)
			assert.Equal(t, want, cfg.Endpoint)
		}
	})

	t.Run("explicit endpoint wins", func(t *testing.T) {
		cfg, err := FromProtoForListing(&pb.RpcAIProviderConfig{Provider: pb.RpcAI_OLLAMA, Endpoint: "http://box:9999/v1"})
		require.NoError(t, err)
		assert.Equal(t, "http://box:9999/v1", cfg.Endpoint)
	})

	t.Run("openai without token is an error", func(t *testing.T) {
		_, err := FromProtoForListing(&pb.RpcAIProviderConfig{Provider: pb.RpcAI_OPENAI})
		require.Error(t, err)
	})

	t.Run("unknown provider without endpoint is an error", func(t *testing.T) {
		_, err := FromProtoForListing(&pb.RpcAIProviderConfig{Provider: pb.RpcAIProvider(99)})
		require.Error(t, err)
	})

	t.Run("openai key over plain http to a remote host is refused", func(t *testing.T) {
		_, err := FromProtoForListing(&pb.RpcAIProviderConfig{
			Provider: pb.RpcAI_OPENAI, Token: "sk-x", Endpoint: "http://proxy.example.com/v1",
		})
		require.ErrorContains(t, err, "plain http")
	})

	t.Run("openai with token", func(t *testing.T) {
		cfg, err := FromProtoForListing(&pb.RpcAIProviderConfig{Provider: pb.RpcAI_OPENAI, Token: "sk-x"})
		require.NoError(t, err)
		assert.Equal(t, "https://api.openai.com/v1", cfg.Endpoint)
		assert.Equal(t, "sk-x", cfg.Token)
	})
}
