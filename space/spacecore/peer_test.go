package spacecore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddSchema(t *testing.T) {
	t.Run("local peer addrs get yamux scheme only", func(t *testing.T) {
		// given
		s := &service{}
		addrs := []string{"192.168.1.5:4242", "10.0.0.3:4242"}
		want := []string{"yamux://192.168.1.5:4242", "yamux://10.0.0.3:4242"}

		// when
		got := s.addSchema(addrs)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("empty input", func(t *testing.T) {
		// given
		s := &service{}

		// when
		got := s.addSchema(nil)

		// then
		assert.Empty(t, got)
	})
}
