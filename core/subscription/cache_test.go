package subscription

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCache(t *testing.T) {
	c := newCache()
	entries := genEntries(3, false)
	for _, e := range entries {
		c.set(e)
		assert.NotNil(t, c.pick(e.id))
		assert.NotNil(t, c.get(e.id))
	}
	for _, e := range entries {
		c.set(&entry{
			id: e.id,
			data: e.data,
		})
		assert.NotNil(t, c.pick(e.id))
	}
	for _, e := range entries {
		c.release(e.id)
	}
	assert.Len(t, c.entries, 0)
}
