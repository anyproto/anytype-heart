package builtintemplate

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	mock_space "github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/testMock/mockSource"
)

func Test_registerBuiltin(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	sourceService := mockSource.NewMockService(ctrl)
	sourceService.EXPECT().NewStaticSource(gomock.Any()).AnyTimes()
	sourceService.EXPECT().RegisterStaticSource(gomock.Any()).AnyTimes()

	marketplaceSpace := mock_space.NewMockSpace(t)
	marketplaceSpace.EXPECT().Id().Return(addr.AnytypeMarketplaceWorkspace)
	marketplaceSpace.EXPECT().Do(mock.Anything, mock.Anything).Return(nil)

	objectStore := objectstore.NewStoreFixture(t)

	builtInTemplates := New()

	ctx := context.Background()
	a := new(app.App)
	a.Register(testutil.PrepareMock(ctx, a, sourceService))
	a.Register(builtInTemplates)
	a.Register(config.New())
	a.Register(objectStore)

	err := builtInTemplates.Init(a)
	assert.NoError(t, err)
	err = builtInTemplates.RegisterBuiltinTemplates(marketplaceSpace)
	assert.NoError(t, err)
}
