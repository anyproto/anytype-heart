package export

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileNamer_Get(t *testing.T) {
	fn := newNamer()
	names := make(map[string]bool)
	nl := []string{
		"some_long_name_12345678901234567890.jpg",
		"some_long_name_12345678901234567890.jpg",
		"some_long_name_12345678901234567890.jpg",
		"one.png",
		"two.png",
		"two.png",
	}
	for i, v := range nl {
		nm := fn.Get(fmt.Sprint(i), v)
		names[nm] = true
		assert.NotEmpty(t, nm, v)
	}
	assert.Equal(t, len(names), len(nl))
}
