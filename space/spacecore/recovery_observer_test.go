package spacecore

import (
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/recovery"
)

type fakeRecovery struct {
	peers []string
	pulls []commonspace.PullEvent
}

func (f *fakeRecovery) Init(*app.App) error { return nil }

func (f *fakeRecovery) Name() string { return recoveryCName }

func (f *fakeRecovery) OnLocalPeerDiscovered(peerId string, _ []string) {
	f.peers = append(f.peers, peerId)
}

func (f *fakeRecovery) ObservePullEvent(ev commonspace.PullEvent) {
	f.pulls = append(f.pulls, ev)
}

type wrongTypeComponent struct{}

func (wrongTypeComponent) Init(*app.App) error { return nil }

func (wrongTypeComponent) Name() string { return recoveryCName }

func TestLookupPeerDiscoveryObserver(t *testing.T) {
	t.Run("constant mirrors recovery.CName", func(t *testing.T) {
		assert.Equal(t, recovery.CName, recoveryCName)
	})

	t.Run("absent component yields nil", func(t *testing.T) {
		assert.Nil(t, lookupPeerDiscoveryObserver(new(app.App)))
	})

	t.Run("component of the wrong type yields nil", func(t *testing.T) {
		a := new(app.App)
		a.Register(wrongTypeComponent{})
		assert.Nil(t, lookupPeerDiscoveryObserver(a))
	})

	t.Run("registered tracker is returned", func(t *testing.T) {
		// given
		a := new(app.App)
		fake := &fakeRecovery{}
		a.Register(fake)

		// when
		observer := lookupPeerDiscoveryObserver(a)

		// then
		assert.Equal(t, fake, observer)
	})
}

func TestLookupPullObserver(t *testing.T) {
	t.Run("absent component yields nil", func(t *testing.T) {
		assert.Nil(t, lookupPullObserver(new(app.App)))
	})

	t.Run("component of the wrong type yields nil", func(t *testing.T) {
		a := new(app.App)
		a.Register(wrongTypeComponent{})
		assert.Nil(t, lookupPullObserver(a))
	})

	t.Run("registered tracker is returned and receives pull events", func(t *testing.T) {
		// given
		a := new(app.App)
		fake := &fakeRecovery{}
		a.Register(fake)

		// when
		observer := lookupPullObserver(a)
		observer.ObservePullEvent(commonspace.PullEvent{Kind: commonspace.PullEventWaiting, SpaceId: "s1"})

		// then
		assert.Equal(t, []commonspace.PullEvent{{Kind: commonspace.PullEventWaiting, SpaceId: "s1"}}, fake.pulls)
	})
}
