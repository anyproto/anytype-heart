package localdiscovery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/net/addrs"
)

type stubNotifierProvider struct{}

func (stubNotifierProvider) Provide(ProviderNotifier, int, string, string) {}
func (stubNotifierProvider) Remove()                                       {}

func newProviderFixture(t *testing.T) (*providerDiscovery, *[]DiscoveryPossibility) {
	l := newProvider()
	l.peerId = "self"
	var states []DiscoveryPossibility
	l.RegisterDiscoveryPossibilityHook(func(state DiscoveryPossibility) {
		states = append(states, state)
	})
	return l, &states
}

func TestProviderDiscovery_PolicyDenied(t *testing.T) {
	t.Run("a policy-denied error reports the local network as restricted", func(t *testing.T) {
		// given
		l, states := newProviderFixture(t)

		// when
		l.DiscoveryError(dnsServicePolicyDenied, "denied")

		// then
		assert.Equal(t, []DiscoveryPossibility{DiscoveryLocalNetworkRestricted}, *states)
	})

	t.Run("other platform errors do not change the state", func(t *testing.T) {
		// given
		l, states := newProviderFixture(t)

		// when
		l.DiscoveryError(-65563, "service not running")

		// then
		assert.Empty(t, *states)
	})

	t.Run("code 0 clears a denial", func(t *testing.T) {
		// given
		l, states := newProviderFixture(t)
		l.DiscoveryError(dnsServicePolicyDenied, "denied")

		// when
		l.DiscoveryError(0, "")

		// then
		require.Len(t, *states, 2)
		assert.NotEqual(t, DiscoveryLocalNetworkRestricted, (*states)[1])
	})

	t.Run("a discovered peer clears a denial", func(t *testing.T) {
		// given
		l, states := newProviderFixture(t)
		l.DiscoveryError(dnsServicePolicyDenied, "denied")

		// when
		l.PeerDiscovered(context.Background(), DiscoveredPeer{PeerId: "peer2", Addrs: []string{"10.0.0.2:4006"}}, OwnAddresses{})

		// then
		require.Len(t, *states, 2)
		assert.NotEqual(t, DiscoveryLocalNetworkRestricted, (*states)[1])
	})

	t.Run("an interface change does not lift a denial", func(t *testing.T) {
		// given
		l, states := newProviderFixture(t)
		l.DiscoveryError(dnsServicePolicyDenied, "denied")
		l.interfacesAddrs = addrs.InterfacesAddrs{} // forces refreshInterfaces to see a change

		// when
		require.NoError(t, l.refreshInterfaces(context.Background()))

		// then
		for _, state := range *states {
			assert.Equal(t, DiscoveryLocalNetworkRestricted, state)
		}
	})

	t.Run("a lost peer changes nothing", func(t *testing.T) {
		// given
		l, states := newProviderFixture(t)

		// when
		l.PeerLost("peer2")

		// then
		assert.Empty(t, *states)
	})
}
