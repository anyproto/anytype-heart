//go:build !android

package localdiscovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewForInstalledProvider(t *testing.T) {
	t.Run("an installed proxy selects the provider-backed discovery", func(t *testing.T) {
		// given
		SetNotifierProvider(stubNotifierProvider{})
		t.Cleanup(func() { SetNotifierProvider(nil) })

		// when
		got := newForInstalledProvider()

		// then
		assert.IsType(t, &providerDiscovery{}, got)
	})

	t.Run("no proxy falls back to zeroconf", func(t *testing.T) {
		// given
		SetNotifierProvider(nil)

		// when
		got := newForInstalledProvider()

		// then
		assert.IsType(t, &zeroconfDiscovery{}, got)
	})
}
