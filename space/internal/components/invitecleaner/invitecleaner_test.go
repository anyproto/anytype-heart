package invitecleaner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStartDelay(t *testing.T) {
	// an account can have hundreds of spaces, which all load at once. The delay is drawn per space so
	// that their coordinator requests do not all land in the same second.
	seen := map[int64]struct{}{}
	for range 100 {
		delay := startDelay()

		assert.GreaterOrEqual(t, delay, startDelayMin)
		assert.Less(t, delay, startDelayMax)
		seen[int64(delay)] = struct{}{}
	}
	assert.Greater(t, len(seen), 1, "the delay must not be the same for every space")
}
