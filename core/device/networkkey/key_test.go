package networkkey

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestKey_SameNetwork pins the rule that a part missing on one side is
// absence of evidence. The three parts of an identity arrive at different
// times — the interface snapshot within milliseconds of start, the client's
// report only once the account app is up — so comparing whole keys read one
// network as two and threw away the verdict learned on it.
func TestKey_SameNetwork(t *testing.T) {
	full := Key{Reported: true, Type: 0, PathId: "en0|192.168.1.1", Snapshot: "192.168.1.218"}

	t.Run("the cold-start half of a key still matches the whole", func(t *testing.T) {
		// given: what the startup check sees before the client has reported
		half := Key{Snapshot: "192.168.1.218"}

		// then
		assert.True(t, full.SameNetwork(half))
		assert.True(t, half.SameNetwork(full))
	})

	t.Run("a genuinely different network still differs", func(t *testing.T) {
		assert.False(t, full.SameNetwork(Key{Snapshot: "10.0.0.5"}))
		assert.False(t, full.SameNetwork(Key{Reported: true, PathId: "en0|10.0.0.1", Snapshot: "192.168.1.218"}))
	})

	t.Run("a reported type change is a different network", func(t *testing.T) {
		wifi := Key{Reported: true, Type: 0, PathId: "p"}
		cell := Key{Reported: true, Type: 1, PathId: "p"}
		assert.False(t, wifi.SameNetwork(cell))
	})

	t.Run("no overlap keeps what we have", func(t *testing.T) {
		// nothing can be proven either way, and deleting a verdict on no
		// evidence is the failure this exists to prevent
		assert.True(t, Key{Snapshot: "a"}.SameNetwork(Key{Reported: true, PathId: "b"}))
	})

	t.Run("a bare type identifies nothing", func(t *testing.T) {
		// "some Wi-Fi" matches every Wi-Fi
		assert.False(t, Key{Reported: true, Type: 0}.Known())
		assert.True(t, Key{Snapshot: "a"}.Known())
		assert.True(t, Key{PathId: "p"}.Known())
	})
}
