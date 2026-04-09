package personalfavorites

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveOrder(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		assert.Nil(t, resolveOrder(nil))
		assert.Nil(t, resolveOrder([]WidgetEntry{}))
	})

	t.Run("linear chain", func(t *testing.T) {
		entries := []WidgetEntry{
			{Id: "b", AfterId: "a"},
			{Id: "c", AfterId: "b"},
			{Id: "a", AfterId: ""},
		}

		got := idsOf(resolveOrder(entries))

		assert.Equal(t, []string{"a", "b", "c"}, got)
	})

	t.Run("no head returns input unchanged", func(t *testing.T) {
		entries := []WidgetEntry{
			{Id: "a", AfterId: "missing"},
			{Id: "b", AfterId: "a"},
		}

		assert.Equal(t, entries, resolveOrder(entries))
	})

	t.Run("duplicate afterId: walker takes first, orphans appended", func(t *testing.T) {
		entries := []WidgetEntry{
			{Id: "a", AfterId: ""},
			{Id: "b", AfterId: "a"},
			{Id: "c", AfterId: "a"},
		}

		got := idsOf(resolveOrder(entries))

		assert.Len(t, got, 3)
		assert.Equal(t, "a", got[0])
		assert.Equal(t, "b", got[1])
		assert.Contains(t, got, "c")
	})

	t.Run("seen-set guards against infinite loop", func(t *testing.T) {
		entries := []WidgetEntry{
			{Id: "a", AfterId: ""},
			{Id: "b", AfterId: "a"},
		}

		assert.Len(t, resolveOrder(entries), 2)
	})
}

func idsOf(entries []WidgetEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Id
	}
	return out
}
