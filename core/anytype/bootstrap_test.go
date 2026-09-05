package anytype

import (
	"slices"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/peerservice"
	"github.com/anyproto/any-sync/net/quicdemotion"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/device"
	"github.com/anyproto/anytype-heart/net/transportpenalty"
)

// registeredNames returns the component list Bootstrap produces. Bootstrap
// only registers (Init and Run happen in app.Start), so the wiring can be
// inspected without starting anything.
func registeredNames() []string {
	a := new(app.App)
	Bootstrap(a)
	return a.ComponentNames()
}

func TestBootstrap_QuicDemotionWiring(t *testing.T) {
	t.Run("enabled by default: quicdemotion is registered before its users", func(t *testing.T) {
		// given
		t.Setenv(transportpenalty.DisableEnv, "")

		// when
		names := registeredNames()

		// then
		demotion := slices.Index(names, quicdemotion.CName)
		penalties := slices.Index(names, transportpenalty.CName)
		peers := slices.Index(names, peerservice.CName)
		network := slices.Index(names, device.CName)
		require.NotEqual(t, -1, demotion, "quicdemotion not registered: auto-demotion is silently off")
		require.NotEqual(t, -1, penalties, "transportpenalty not registered: verdicts are never persisted")
		require.NotEqual(t, -1, peers)
		require.NotEqual(t, -1, network)
		// peerservice picks quicdemotion up in its own Init
		assert.Less(t, demotion, peers, "quicdemotion must be registered before peerservice")
		// transportpenalty seeds quicdemotion in Init. Seed before
		// quicdemotion's own Init panics on a nil map, and app.MustComponent
		// resolves by type regardless of order, so only the order guards it.
		assert.Less(t, demotion, penalties, "quicdemotion must be registered before transportpenalty")
		// transportpenalty hooks into networkState and, closing first
		// (reverse order), guards the hook before the monitor's last recoveries
		assert.Less(t, network, penalties, "networkState must be registered before transportpenalty")
	})
	t.Run("kill switch: ANYTYPE_QUIC_AUTO_DEMOTION=0 leaves quicdemotion out", func(t *testing.T) {
		// given
		t.Setenv(transportpenalty.DisableEnv, "0")

		// when
		names := registeredNames()

		// then
		assert.NotContains(t, names, quicdemotion.CName)
		// the persistence component stays registered and disables itself
		assert.Contains(t, names, transportpenalty.CName)
	})
	t.Run("any other value keeps auto-demotion on", func(t *testing.T) {
		// given
		t.Setenv(transportpenalty.DisableEnv, "1")

		// when
		names := registeredNames()

		// then
		assert.Contains(t, names, quicdemotion.CName)
	})
}
