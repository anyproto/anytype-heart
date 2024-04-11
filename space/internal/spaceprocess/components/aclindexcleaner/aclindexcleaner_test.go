package aclindexcleaner

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/internal/components/dependencies/mock_dependencies"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus/mock_spacestatus"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

var ctx = context.Background()

func TestAclIndexCleaner(t *testing.T) {
	_ = newFixture(t)
}

type fixture struct {
	*aclIndexCleaner
	a       *app.App
	indexer *mock_dependencies.MockSpaceIndexer
	status  *mock_spacestatus.MockSpaceStatus
}

func newFixture(t *testing.T) *fixture {
	fx := &fixture{
		aclIndexCleaner: New().(*aclIndexCleaner),
		a:               new(app.App),
		indexer:         mock_dependencies.NewMockSpaceIndexer(t),
		status:          mock_spacestatus.NewMockSpaceStatus(t),
	}
	fx.a.Register(testutil.PrepareMock(ctx, fx.a, fx.status)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.indexer)).
		Register(fx)

	fx.status.EXPECT().SpaceId().Return("spaceId")
	fx.indexer.EXPECT().RemoveAclIndexes("spaceId").Return(nil)
	require.NoError(t, fx.a.Start(ctx))
	return fx
}
