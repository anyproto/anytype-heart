package domain

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIntegrationNameCtx(t *testing.T) {
	t.Run("roundtrip", func(t *testing.T) {
		ctx := CtxWithIntegrationName(context.Background(), "Claude Desktop")
		assert.Equal(t, "Claude Desktop", IntegrationNameFromCtx(ctx))
	})

	t.Run("the value is the RAW name — never normalized", func(t *testing.T) {
		// the F2/F3 root fix: names that the old slug collapsed ("Claude
		// Desktop" vs "Claude/Desktop" → claude-desktop) or erased ("日本語アプリ"
		// → "") must ride the carrier byte-for-byte. Each row fails if any
		// normalization creeps back into the carrier.
		for _, name := range []string{"Claude/Desktop", "CLAUDE DESKTOP", "日本語アプリ", "🙂", "!!!"} {
			ctx := CtxWithIntegrationName(context.Background(), name)
			assert.Equal(t, name, IntegrationNameFromCtx(ctx), "name %q must not be rewritten", name)
		}
	})

	t.Run("absent is empty", func(t *testing.T) {
		assert.Equal(t, "", IntegrationNameFromCtx(context.Background()))
	})

	t.Run("nil ctx is empty, not a panic", func(t *testing.T) {
		// the creation pipeline sees nil ctx from tests and internal callers
		assert.Equal(t, "", IntegrationNameFromCtx(nil))
	})

	t.Run("empty name installs nothing", func(t *testing.T) {
		// empty AppName ⇒ no stamp (§5): absence and empty stay one state
		ctx := CtxWithIntegrationName(context.Background(), "")
		assert.Equal(t, "", IntegrationNameFromCtx(ctx))
	})
}
