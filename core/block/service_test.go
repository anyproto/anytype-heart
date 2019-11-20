package block

import (
	"errors"
	"testing"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_OpenBlock(t *testing.T) {
	t.Run("error while open block", func(t *testing.T) {
		var (
			accountId = "123"
			blockId   = "456"
			expErr    = errors.New("test err")
		)
		fx := newFixture(t, accountId)
		defer fx.ctrl.Finish()
		defer fx.tearDown()

		fx.anytype.EXPECT().GetBlock(blockId).Return(nil, expErr)

		err := fx.OpenBlock(blockId)
		require.Equal(t, expErr, err)
	})
	t.Run("should open dashboard", func(t *testing.T) {
		var (
			accountId = "123"
			blockId   = "456"
		)
		fx := newFixture(t, accountId)
		defer fx.ctrl.Finish()
		defer fx.tearDown()

		mb, _ := fx.newMockBlockWithContent(blockId, &model.BlockContentOfDashboard{
			Dashboard: &model.BlockContentDashboard{},
		}, nil, nil)

		fx.anytype.EXPECT().GetBlock(blockId).Return(mb, nil)

		err := fx.OpenBlock(blockId)
		require.NoError(t, err)
		defer func() { require.NoError(t, fx.CloseBlock(blockId)) }()

		assert.Len(t, fx.events, 1)
		assert.Equal(t, smartBlockTypeDashboard, fx.Service.(*service).smartBlocks[blockId].Type())

	})
	t.Run("should open page", func(t *testing.T) {
		var (
			accountId = "123"
			blockId   = "456"
		)
		fx := newFixture(t, accountId)
		defer fx.ctrl.Finish()
		defer fx.tearDown()

		mb, _ := fx.newMockBlockWithContent(blockId, &model.BlockContentOfPage{
			Page: &model.BlockContentPage{},
		}, nil, nil)

		fx.anytype.EXPECT().GetBlock(blockId).Return(mb, nil)

		err := fx.OpenBlock(blockId)
		require.NoError(t, err)
		defer func() { require.NoError(t, fx.CloseBlock(blockId)) }()

		assert.Len(t, fx.events, 1)
		assert.Equal(t, smartBlockTypePage, fx.Service.(*service).smartBlocks[blockId].Type())
	})
	t.Run("should replace home id to real", func(t *testing.T) {
		var (
			accountId  = "123"
			blockId    = "home"
			realHomeId = "realHomeId"
		)
		fx := newFixture(t, accountId)
		defer fx.ctrl.Finish()
		defer fx.tearDown()

		mb, _ := fx.newMockBlockWithContent(realHomeId, &model.BlockContentOfPage{
			Page: &model.BlockContentPage{},
		}, nil, nil)

		fx.anytype.EXPECT().GetBlock(realHomeId).Return(mb, nil)

		err := fx.OpenBlock(blockId)
		require.NoError(t, err)
		defer func() { require.NoError(t, fx.CloseBlock(blockId)) }()

		assert.Len(t, fx.events, 1)
		assert.Equal(t, smartBlockTypePage, fx.Service.(*service).smartBlocks[realHomeId].Type())
	})
}

func newFixture(t *testing.T, accountId string) *fixture {
	ctrl := gomock.NewController(t)
	anytype := testMock.NewMockAnytype(ctrl)
	fx := &fixture{
		t:       t,
		ctrl:    ctrl,
		anytype: anytype,
	}
	fx.Service = NewService(accountId, anytype, fx.sendEvent)
	return fx
}

type fixture struct {
	Service
	t       *testing.T
	ctrl    *gomock.Controller
	anytype *testMock.MockAnytype
	events  []*pb.Event
}

func (fx *fixture) sendEvent(e *pb.Event) {
	fx.events = append(fx.events, e)
}

func (fx *fixture) newMockBlockWithContent(id string, content model.IsBlockContent, childrenIds []string, db map[string]core.BlockVersion) (b *blockWrapper, v *testMock.MockBlockVersion) {
	if db == nil {
		db = make(map[string]core.BlockVersion)
	}
	v = fx.newMockVersion(&model.Block{
		Id:          id,
		Content:     content,
		ChildrenIds: childrenIds,
	})
	v.EXPECT().DependentBlocks().AnyTimes().Return(db)
	b = &blockWrapper{MockBlock: testMock.NewMockBlock(fx.ctrl)}
	b.EXPECT().GetId().AnyTimes().Return(id)
	b.EXPECT().GetCurrentVersion().AnyTimes().Return(v, nil)
	fx.anytype.EXPECT().PredefinedBlockIds().AnyTimes().Return(core.PredefinedBlockIds{Home: "realHomeId"})
	return
}

func (fx *fixture) newMockVersion(m *model.Block) (v *testMock.MockBlockVersion) {
	v = testMock.NewMockBlockVersion(fx.ctrl)
	v.EXPECT().Model().AnyTimes().Return(m)
	return
}

func (fx *fixture) tearDown() {
	require.NoError(fx.t, fx.Close())
}

type matcher struct {
	name string
	f    func(x interface{}) bool
}

func (m *matcher) Matches(x interface{}) bool {
	return m.f(x)
}

func (m *matcher) String() string {
	return m.name
}
