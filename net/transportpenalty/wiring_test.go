package transportpenalty

import (
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/quicdemotion"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/anyproto/any-sync/nodeconf/mock_nodeconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newNodeConf is the nodeconf stub the real quicdemotion component needs: it
// resolves nodeconf by name at Init and asks it for node types when seeding.
func newNodeConf(t *testing.T) app.Component {
	ctrl := gomock.NewController(t)
	nc := mock_nodeconf.NewMockService(ctrl)
	nc.EXPECT().Name().Return(nodeconf.CName).AnyTimes()
	nc.EXPECT().Init(gomock.Any()).AnyTimes()
	nc.EXPECT().Run(gomock.Any()).AnyTimes()
	nc.EXPECT().Close(gomock.Any()).AnyTimes()
	nc.EXPECT().NodeTypes(gomock.Any()).Return(nil).AnyTimes()
	return nc
}

// TestService_RealQuicDemotion drives the real any-sync component instead of
// the double. The double cannot catch a mismatch between what this component
// writes and what quicdemotion accepts, nor the Init-order requirement.
func TestService_RealQuicDemotion(t *testing.T) {
	stored := storedState{NetworkKey: "net-A", UpdatedAt: time.Now(), Penalties: demotedPeers("p1")}
	t.Run("registered after quicdemotion: the stored verdict is seeded", func(t *testing.T) {
		// given
		repo := t.TempDir()
		svc := New().(*service)
		svc.startupCheckInterval = time.Hour
		demotion := quicdemotion.New()
		a := new(app.App)
		a.Register(&fakeWallet{repoPath: repo}).
			Register(newNodeConf(t)).
			Register(demotion).
			Register(&fakeNetwork{identity: "net-A"}).
			Register(svc)
		(&fixture{service: svc, a: a, repo: repo}).writeStateFile(t, stored)

		// when
		require.NoError(t, a.Start(ctx))
		defer func() { require.NoError(t, a.Close(ctx)) }()

		// then
		assert.Contains(t, demotion.Snapshot().Peers, "p1")
	})
	t.Run("registered before quicdemotion: Start panics", func(t *testing.T) {
		// Init runs in registration order and app.MustComponent resolves by
		// type regardless of it, so swapping the two Register calls in
		// bootstrap is caught only by this and core/anytype's wiring test:
		// Seed and SetObserver run on quicdemotion's not-yet-created state.
		repo := t.TempDir()
		svc := New().(*service)
		a := new(app.App)
		a.Register(&fakeWallet{repoPath: repo}).
			Register(newNodeConf(t)).
			Register(&fakeNetwork{identity: "net-A"}).
			Register(svc).
			Register(quicdemotion.New())
		(&fixture{service: svc, a: a, repo: repo}).writeStateFile(t, stored)

		// when / then
		assert.Panics(t, func() { _ = a.Start(ctx) })
	})
}
