package store

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestItem(t *testing.T) {
	good := Item{V: 1, Rev: 7, Key: "abcDEF123+/=", Account: "A5k2d9sFZw84yisTxRnz2bPRd1YPfVfhxqymZ6yESprFTG65", Port: 31009, Tier: "full"}

	t.Run("the stored form is the documented one and round-trips", func(t *testing.T) {
		raw, err := good.Encode()
		require.NoError(t, err)
		assert.JSONEq(t, `{"v":1,"rev":7,"key":"abcDEF123+/=","account":"A5k2d9sFZw84yisTxRnz2bPRd1YPfVfhxqymZ6yESprFTG65","port":31009,"tier":"full"}`, string(raw))
		back, err := DecodeItem(raw)
		require.NoError(t, err)
		assert.Equal(t, good, back)
		assert.Less(t, len(raw), 2560, "fits a Windows credential blob")
	})

	t.Run("the largest item still fits a Windows credential blob", func(t *testing.T) {
		big := good
		big.Key = strings.Repeat("k", MaxKeyBytes)
		big.Account = strings.Repeat("a", 256)
		big.Rev = 1 << 60
		raw, err := big.Encode()
		require.NoError(t, err)
		assert.Less(t, len(raw), 2560)
	})

	t.Run("the fingerprint is the registry's: the first 16 hex of sha256(key)", func(t *testing.T) {
		// printf %s hello | shasum -a 256
		assert.Equal(t, "2cf24dba5fb0a30e", Fingerprint("hello"))
		assert.Equal(t, Fingerprint(good.Key)+"@7", good.Stamp())
	})

	t.Run("invalid items are refused both ways and the error never carries the key", func(t *testing.T) {
		for name, mutate := range map[string]func(*Item){
			"version":       func(i *Item) { i.V = 2 },
			"rev":           func(i *Item) { i.Rev = 0 },
			"no key":        func(i *Item) { i.Key = "" },
			"key newline":   func(i *Item) { i.Key = "SECRETa\nb" },
			"key space":     func(i *Item) { i.Key = "SECRETa b" },
			"key non-ascii": func(i *Item) { i.Key = "SECRETé" },
			"key too long":  func(i *Item) { i.Key = strings.Repeat("S", MaxKeyBytes+1) },
			"no account":    func(i *Item) { i.Account = "" },
			"port low":      func(i *Item) { i.Port = 0 },
			"port high":     func(i *Item) { i.Port = 65536 },
			"tier":          func(i *Item) { i.Tier = "medium" },
		} {
			it := good
			mutate(&it)
			_, err := it.Encode()
			require.Error(t, err, name)
			assert.NotContains(t, err.Error(), "SECRET", name)
		}
		for _, raw := range []string{``, `garbage`, `[]`, `{"v":1}`, `{"v":1,"rev":1,"key":"SECRET\n","account":"a","port":1,"tier":"full"}`} {
			_, err := DecodeItem([]byte(raw))
			require.ErrorIs(t, err, ErrInvalidItem, raw)
			assert.NotContains(t, err.Error(), "SECRET")
		}
	})

	t.Run("connection ids", func(t *testing.T) {
		for _, id := range []string{"claude-desktop", "other-1a2b3c4d", "a", strings.Repeat("a", 64)} {
			assert.True(t, ValidConnectionID(id), id)
		}
		for _, id := range []string{"", "A", "a_b", "a/b", "../x", "a b", strings.Repeat("a", 65)} {
			assert.False(t, ValidConnectionID(id), id)
		}
	})
}

func TestNamespaces(t *testing.T) {
	assert.Equal(t, "Anytype MCP", ServiceName(NamespaceRelease))
	assert.Equal(t, "Anytype MCP Dev", ServiceName(NamespaceDev))
	assert.Equal(t, "Anytype MCP Dev", ServiceName(""), "development is the default")
	assert.Equal(t, "Anytype MCP Dev", ServiceName("Release"))
	assert.Equal(t, "anytype-mcp", SecretServiceName(NamespaceRelease))
	assert.Equal(t, "anytype-mcp-dev", SecretServiceName("x"))
	assert.False(t, TestBuild)
}

func TestFileStore(t *testing.T) {
	ctx := context.Background()
	f := &File{Dir: t.TempDir()}
	_, err := f.Read(ctx, "a", false)
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, f.Delete(ctx, "a"), ErrNotFound)
	require.NoError(t, f.Write(ctx, "a", []byte("one"), false))
	require.NoError(t, f.Write(ctx, "a", []byte("two"), false))
	raw, err := f.Read(ctx, "a", false)
	require.NoError(t, err)
	assert.Equal(t, "two", string(raw))
	require.NoError(t, f.Delete(ctx, "a"))
	require.NoError(t, f.Probe(ctx))
}
