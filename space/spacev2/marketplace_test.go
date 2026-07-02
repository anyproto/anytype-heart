package spacev2

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies/mock_dependencies"
	"github.com/anyproto/anytype-heart/space/techspace/mock_techspace"
)

func newGetFixture(t *testing.T) *service {
	return &service{
		registry:        newRegistry(),
		personalSpaceId: testPersonalSpaceId,
		techSpaceId:     testTechSpaceId,
		techSpaceReady:  make(chan struct{}),
	}
}

func TestService_GetMarketplace(t *testing.T) {
	t.Run("marketplace resolves via static entry, reindexes once", func(t *testing.T) {
		// given
		s := newGetFixture(t)
		vs := mock_clientspace.NewMockSpace(t)
		indexer := mock_dependencies.NewMockSpaceIndexer(t)
		indexer.EXPECT().ReindexMarketplaceSpace(vs).Return(nil).Times(1)
		mp := &marketplaceController{vs: vs, indexer: indexer}
		s.registry.addStatic(addr.AnytypeMarketplaceWorkspace, mp)

		// when: two gets, one reindex
		got1, err1 := s.Get(ctx, addr.AnytypeMarketplaceWorkspace)
		got2, err2 := s.Get(ctx, addr.AnytypeMarketplaceWorkspace)

		// then
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Same(t, vs, got1)
		assert.Same(t, vs, got2)
	})
}

func TestService_GetTechSpace(t *testing.T) {
	t.Run("blocks until tech space ready", func(t *testing.T) {
		// given
		s := newGetFixture(t)

		// not resolved yet: deadline
		shortCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
		defer cancel()
		_, err := s.Get(shortCtx, testTechSpaceId)
		require.ErrorIs(t, err, context.DeadlineExceeded)

		// resolved: returns the tech space
		fx := &resolveFixture{techMock: mock_techspace.NewMockTechSpace(t)}
		s.techSpace = fx.techSpaceFor()
		close(s.techSpaceReady)
		got, err := s.Get(ctx, testTechSpaceId)
		require.NoError(t, err)
		assert.NotNil(t, got)
	})
}

func TestService_GetUnknown(t *testing.T) {
	t.Run("unknown space is not-exists", func(t *testing.T) {
		s := newGetFixture(t)
		_, err := s.Get(ctx, "unknown.space")
		require.ErrorIs(t, err, ErrSpaceNotExists)
	})
}

func TestService_WaitUnknown(t *testing.T) {
	t.Run("wait for a space without a view is not-exists", func(t *testing.T) {
		// given: tech space resolved, no view for the id
		s := newGetFixture(t)
		techMock := mock_techspace.NewMockTechSpace(t)
		techMock.EXPECT().SpaceViewExists(ctx, "unknown.space").Return(false, nil)
		fx := &resolveFixture{techMock: techMock}
		s.techSpace = fx.techSpaceFor()
		close(s.techSpaceReady)

		// when
		_, err := s.Wait(ctx, "unknown.space")

		// then
		require.ErrorIs(t, err, ErrSpaceNotExists)
	})
}
