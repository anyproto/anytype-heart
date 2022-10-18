package editor

import (
	"context"
	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/anytypeio/go-anytype-middleware/util/testMock/mockDoc"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBreadcrumbs_Init(t *testing.T) {
	fx := newFixture(t)
	defer fx.Finish()
	fx.expectDerivedDetails()
	fx.mockDoc.EXPECT().ReportChange(gomock.Any(), gomock.Any())
	b := NewBreadcrumbs()
	err := b.Init(&smartblock.InitContext{
		App:    fx.app.App,
		Source: source.NewVirtual(fx.mockAnytype, model.SmartBlockType_Breadcrumbs),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, b.Id())
	assert.NotEmpty(t, b.RootId())
	assert.Len(t, b.Blocks(), 1)
}

func TestBreadcrumbs_SetCrumbs(t *testing.T) {
	t.Run("set ids", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.Finish()
		fx.expectDerivedDetails()
		fx.mockDoc.EXPECT().ReportChange(gomock.Any(), gomock.Any()).AnyTimes()

		b := NewBreadcrumbs()
		err := b.Init(&smartblock.InitContext{
			App:    fx.app.App,
			Source: source.NewVirtual(fx.mockAnytype, model.SmartBlockType_Breadcrumbs),
		})
		require.NoError(t, err)
		require.NoError(t, b.SetCrumbs([]string{"one", "two"}))
		require.Len(t, b.NewState().Pick(b.RootId()).Model().ChildrenIds, 2)
		require.NoError(t, b.SetCrumbs([]string{"one", "two", "three"}))
		require.Len(t, b.NewState().Pick(b.RootId()).Model().ChildrenIds, 3)
		require.NoError(t, b.SetCrumbs([]string{"next"}))
		require.Len(t, b.NewState().Pick(b.RootId()).Model().ChildrenIds, 1)
	})
}

func newFixture(t *testing.T) *fixture {
	fx := &fixture{
		ctrl: gomock.NewController(t),
		app:  testapp.New(),
		t:    t,
	}
	fx.mockStore = testMock.RegisterMockObjectStore(fx.ctrl, fx.app)
	fx.mockDoc = mockDoc.RegisterMockDoc(fx.ctrl, fx.app)
	fx.mockAnytype = testMock.RegisterMockAnytype(fx.ctrl, fx.app)
	fx.app.Register(restriction.New())
	require.NoError(t, fx.app.Start(context.Background()))
	return fx
}

type fixture struct {
	t           *testing.T
	ctrl        *gomock.Controller
	app         *testapp.TestApp
	mockStore   *testMock.MockObjectStore
	mockDoc     *mockDoc.MockService
	mockAnytype *testMock.MockService
}

func (fx *fixture) expectDerivedDetails() {
	fx.mockStore.EXPECT().GetDetails(gomock.Any()).Return(&model.ObjectDetails{}, nil)
	fx.mockStore.EXPECT().GetPendingLocalDetails(gomock.Any()).Return(&model.ObjectDetails{}, nil)
	fx.mockStore.EXPECT().UpdatePendingLocalDetails(gomock.Any(), gomock.Any())
}

func (fx *fixture) Finish() {
	assert.NoError(fx.t, fx.app.Close())
	fx.ctrl.Finish()
}
