package system_object

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/system_object/mock_system_object"
	"github.com/anyproto/anytype-heart/pkg/lib/core/mock_core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type fixture struct {
	objectStore *objectstore.StoreFixture

	Service
}

func newFixture(t *testing.T) *fixture {
	objectStore := objectstore.NewStoreFixture(t)
	coreService := mock_core.NewMockService(t)
	deriver := mock_system_object.NewMockderiver(t)

	ctx := context.Background()
	a := new(app.App)
	a.Register(objectStore)
	a.Register(testutil.PrepareMock(ctx, a, coreService))
	a.Register(testutil.PrepareMock(ctx, a, deriver))

	s := New()
	err := s.Init(a)
	require.NoError(t, err)
	return &fixture{
		Service:     s,
		objectStore: objectStore,
	}
}
