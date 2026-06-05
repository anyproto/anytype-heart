package space

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrioritizeSpaceId(t *testing.T) {
	t.Run("preferred space moved to the front", func(t *testing.T) {
		// when
		got := prioritizeSpaceId([]string{"a", "b", "c"}, "b")

		// then: preferred is first, the rest are still present
		assert.Equal(t, "b", got[0])
		assert.ElementsMatch(t, []string{"a", "b", "c"}, got)
		assert.Len(t, got, 3)
	})

	t.Run("empty preferred keeps order", func(t *testing.T) {
		got := prioritizeSpaceId([]string{"a", "b"}, "")
		assert.Equal(t, []string{"a", "b"}, got)
	})

	t.Run("preferred not in list keeps order", func(t *testing.T) {
		got := prioritizeSpaceId([]string{"a", "b"}, "z")
		assert.Equal(t, []string{"a", "b"}, got)
	})
}
