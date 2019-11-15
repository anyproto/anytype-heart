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
		defer fx.tearDown()

		mb, _ := fx.newMockBlockWithContent(blockId, &model.BlockContentOfDashboard{
			Dashboard: &model.BlockContentDashboard{},
		}, nil)

		fx.anytype.EXPECT().GetBlock(blockId).Return(mb, nil)
		mb.EXPECT().SubscribeClientEvents(gomock.Any())

		err := fx.OpenBlock(blockId)
		require.NoError(t, err)
		defer func() { require.NoError(t, fx.CloseBlock(blockId)) }()

		assert.Len(t, fx.events, 1)
		assert.IsType(t, (*dashboard)(nil), fx.Service.(*service).smartBlocks[blockId])

	})
	t.Run("should open page", func(t *testing.T) {
		var (
			accountId = "123"
			blockId   = "456"
		)
		fx := newFixture(t, accountId)
		defer fx.tearDown()

		mb, _ := fx.newMockBlockWithContent(blockId, &model.BlockContentOfPage{
			Page: &model.BlockContentPage{},
		}, nil)

		fx.anytype.EXPECT().GetBlock(blockId).Return(mb, nil)
		mb.EXPECT().SubscribeClientEvents(gomock.Any())

		err := fx.OpenBlock(blockId)
		require.NoError(t, err)
		defer func() { require.NoError(t, fx.CloseBlock(blockId)) }()

		assert.Len(t, fx.events, 1)
		assert.IsType(t, (*page)(nil), fx.Service.(*service).smartBlocks[blockId])
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

func (fx *fixture) newMockBlockWithContent(id string, content model.IsBlockContent, db map[string]core.BlockVersion) (b *testMock.MockBlock, v *testMock.MockBlockVersion) {
	if db == nil {
		db = make(map[string]core.BlockVersion)
	}
	v = testMock.NewMockBlockVersion(fx.ctrl)
	v.EXPECT().Model().AnyTimes().Return(&model.Block{
		Id:      id,
		Content: content,
	})
	v.EXPECT().DependentBlocks().AnyTimes().Return(db)
	b = testMock.NewMockBlock(fx.ctrl)
	b.EXPECT().GetId().AnyTimes().Return(id)
	b.EXPECT().GetCurrentVersion().AnyTimes().Return(v, nil)
	return
}

func (fx *fixture) tearDown() {
	require.NoError(fx.t, fx.Close())
	fx.ctrl.Finish()
}
