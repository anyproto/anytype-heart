package migration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunner(t *testing.T) {
	t.Run("no panic", func(t *testing.T) {
		assert.NotPanics(t, func() { Run(nil, nil) })
	})
}
