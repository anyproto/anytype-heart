package store

import (
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
		assert.Less(t, len(raw), MaxItemBytes)
	})

	t.Run("the largest item fits, whatever the key is made of", func(t *testing.T) {
		// the characters JSON escapes, and the ones HTML escaping would
		// have blown up sixfold
		for _, ch := range []string{`"`, "&", "<", ">", `\`, "k"} {
			big := Item{V: 1, Rev: 1 << 62, Key: strings.Repeat(ch, MaxKeyBytes), Account: strings.Repeat("a", 256), Port: 65535, Tier: "large"}
			raw, err := big.Encode()
			require.NoError(t, err, ch)
			assert.LessOrEqual(t, len(raw), MaxItemBytes, ch)
			back, err := DecodeItem(raw)
			require.NoError(t, err, ch)
			assert.Equal(t, big, back, ch)
		}
		raw, err := Item{V: 1, Rev: 1, Key: "a&b<c>", Account: "a", Port: 1, Tier: "full"}.Encode()
		require.NoError(t, err)
		assert.Contains(t, string(raw), `"a&b<c>"`, "stored without HTML escaping")
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
	assert.Equal(t, "items", FolderName(NamespaceRelease))
	assert.Equal(t, "items-dev", FolderName(NamespaceDev))
	assert.Equal(t, "items-dev", FolderName(""), "development is the default")
	assert.Equal(t, "items-dev", FolderName("Release"))
}

func TestDecodeItemTakesExactlyOneItem(t *testing.T) {
	good := `{"v":1,"rev":1,"key":"k","account":"a","port":1,"tier":"full"}`
	_, err := DecodeItem([]byte(good + "\n"))
	require.NoError(t, err)
	for _, tail := range []string{"x", "{}", good, " 1"} {
		_, err := DecodeItem([]byte(good + tail))
		require.ErrorIs(t, err, ErrInvalidItem, tail)
	}
	_, err = DecodeItem([]byte(good + strings.Repeat(" ", MaxItemBytes)))
	require.ErrorIs(t, err, ErrInvalidItem, "longer than an item can be")
}
