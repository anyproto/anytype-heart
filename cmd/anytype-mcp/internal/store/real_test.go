package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRealStore runs against this OS's real key store. It is opt-in
// (ANYTYPE_MCP_REAL_STORE=1): on a developer machine it touches the
// user's keychain, under a service name of its own and with random
// connection ids that it deletes. CI runs it on native runners.
func TestRealStore(t *testing.T) {
	if os.Getenv("ANYTYPE_MCP_REAL_STORE") == "" {
		t.Skip("set ANYTYPE_MCP_REAL_STORE=1 to run against the real key store")
	}
	ctx := context.Background()
	suffix := make([]byte, 6)
	_, _ = rand.Read(suffix)
	id := "test-" + hex.EncodeToString(suffix)
	// two stores that differ only by namespace, like the two builds
	a, b := newPlatformStore("Anytype MCP Test A"), newPlatformStore("Anytype MCP Test B")
	t.Cleanup(func() {
		_ = a.Delete(ctx, id)
		_ = b.Delete(ctx, id)
	})

	require.NoError(t, a.Probe(ctx))
	_, err := a.Read(ctx, id, false)
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, a.Delete(ctx, id), ErrNotFound)

	require.NoError(t, a.Write(ctx, id, []byte(`{"n":1}`), false))
	raw, err := a.Read(ctx, id, false)
	require.NoError(t, err)
	assert.Equal(t, `{"n":1}`, string(raw))

	// an update replaces the value in place
	require.NoError(t, a.Write(ctx, id, []byte(`{"n":2}`), false))
	raw, err = a.Read(ctx, id, false)
	require.NoError(t, err)
	assert.Equal(t, `{"n":2}`, string(raw))

	// the other namespace can't see it
	_, err = b.Read(ctx, id, false)
	require.ErrorIs(t, err, ErrNotFound)

	require.NoError(t, a.Delete(ctx, id))
	_, err = a.Read(ctx, id, false)
	require.ErrorIs(t, err, ErrNotFound)
}
