package editor

import (
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree/mock_objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/order"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

func TestSpaceView_AccessType(t *testing.T) {
	t.Run("personal", func(t *testing.T) {
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		err := fx.SetAccessType(spaceinfo.AccessTypePersonal)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypePersonal, fx.getAccessType())
		err = fx.SetAccessType(spaceinfo.AccessTypeShared)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypePersonal, fx.getAccessType())
		err = fx.SetAccessType(spaceinfo.AccessTypePrivate)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypePersonal, fx.getAccessType())
		err = fx.SetAclInfo(false, nil, nil, time.Now().Unix())
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypePersonal, fx.getAccessType())
		info := spaceinfo.NewSpaceLocalInfo("spaceId")
		info.SetShareableStatus(spaceinfo.ShareableStatusShareable)
		err = fx.SetSpaceLocalInfo(info)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypePersonal, fx.getAccessType())
	})
	t.Run("private->shareable", func(t *testing.T) {
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		err := fx.SetAccessType(spaceinfo.AccessTypePrivate)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypePrivate, fx.getAccessType())
		err = fx.SetAccessType(spaceinfo.AccessTypeShared)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypeShared, fx.getAccessType())
		err = fx.SetAccessType(spaceinfo.AccessTypePrivate)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypePrivate, fx.getAccessType())
		err = fx.SetAclInfo(false, nil, nil, time.Now().Unix())
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypeShared, fx.getAccessType())
		info := spaceinfo.NewSpaceLocalInfo("spaceId")
		info.SetShareableStatus(spaceinfo.ShareableStatusNotShareable)
		err = fx.SetSpaceLocalInfo(info)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypeShared, fx.getAccessType())
		err = fx.SetAclInfo(true, nil, nil, time.Now().Unix())
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypePrivate, fx.getAccessType())
		info.SetShareableStatus(spaceinfo.ShareableStatusShareable)
		err = fx.SetSpaceLocalInfo(info)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypeShared, fx.getAccessType())
	})
}

func TestSpaceView_Info(t *testing.T) {
	t.Run("local", func(t *testing.T) {
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		firstLocalInfo := fx.GetLocalInfo()
		require.Equal(t, spaceinfo.LocalStatusUnknown, firstLocalInfo.GetLocalStatus())
		require.Equal(t, spaceinfo.RemoteStatusUnknown, firstLocalInfo.GetRemoteStatus())
		info := spaceinfo.NewSpaceLocalInfo("spaceId")
		info.SetLocalStatus(spaceinfo.LocalStatusOk).
			SetRemoteStatus(spaceinfo.RemoteStatusOk).
			SetReadLimit(10).
			SetWriteLimit(10).
			SetShareableStatus(spaceinfo.ShareableStatusShareable)
		err := fx.SetSpaceLocalInfo(info)
		require.NoError(t, err)
		curInfo := fx.GetLocalInfo()
		require.Equal(t, spaceinfo.LocalStatusOk, curInfo.GetLocalStatus())
		require.Equal(t, spaceinfo.RemoteStatusOk, curInfo.GetRemoteStatus())
		require.Equal(t, uint32(10), curInfo.GetReadLimit())
		require.Equal(t, uint32(10), curInfo.GetWriteLimit())
		require.Equal(t, spaceinfo.ShareableStatusShareable, curInfo.GetShareableStatus())
	})
	t.Run("persistent", func(t *testing.T) {
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		info := spaceinfo.NewSpacePersistentInfo("spaceId")
		info.SetAccountStatus(spaceinfo.AccountStatusActive)
		err := fx.SetSpacePersistentInfo(info)
		require.NoError(t, err)
		curInfo := fx.GetPersistentInfo()
		require.Equal(t, spaceinfo.AccountStatusActive, curInfo.GetAccountStatus())
		info = spaceinfo.NewSpacePersistentInfo("spaceId")
		info.SetAclHeadId("aclHeadId")
		err = fx.SetSpacePersistentInfo(info)
		require.NoError(t, err)
		curInfo = fx.GetPersistentInfo()
		require.Equal(t, "aclHeadId", curInfo.GetAclHeadId())
		require.Equal(t, spaceinfo.AccountStatusActive, curInfo.GetAccountStatus())
	})
}

func TestSpaceView_SharedSpacesLimit(t *testing.T) {
	fx := newSpaceViewFixture(t)
	defer fx.finish()
	err := fx.SetSharedSpacesLimit(10)
	require.NoError(t, err)
	require.Equal(t, 10, fx.GetSharedSpacesLimit())
}

func TestSpaceView_SetOwner(t *testing.T) {
	fx := newSpaceViewFixture(t)
	defer fx.finish()
	err := fx.SetOwner("ownerId", 125)
	require.NoError(t, err)
	require.Equal(t, "ownerId", fx.CombinedDetails().GetString(bundle.RelationKeyCreator))
	require.Equal(t, int64(125), fx.CombinedDetails().GetInt64(bundle.RelationKeyCreatedDate))
}

func TestSpaceView_SetAfterOrder(t *testing.T) {
	t.Run("set view after given id", func(t *testing.T) {
		// given
		fx := newSpaceViewFixture(t)
		defer fx.finish()

		// when
		err := fx.SetAfterOrder("viewOrderId")

		// then
		require.NoError(t, err)
		assert.NotEmpty(t, fx.Details().GetString(bundle.RelationKeySpaceOrder))
	})
	t.Run("set view after given id, order exist", func(t *testing.T) {
		// given
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		state := fx.NewState()
		state.SetDetail(bundle.RelationKeySpaceOrder, domain.String("spaceViewOrderId"))
		err := fx.Apply(state)
		require.NoError(t, err)

		// when
		err = fx.SetAfterOrder("viewOrderId")

		// then
		require.NoError(t, err)
		assert.NotEqual(t, "spaceViewOrderId", fx.Details().GetString(bundle.RelationKeySpaceOrder))
		assert.True(t, fx.Details().GetString(bundle.RelationKeySpaceOrder) > "viewOrderId")
	})
	t.Run("set view after given id, order exist, but already less than given view", func(t *testing.T) {
		// given
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		state := fx.NewState()
		state.SetDetail(bundle.RelationKeySpaceOrder, domain.String("viewOrderId"))
		err := fx.Apply(state)
		require.NoError(t, err)

		// when
		err = fx.SetAfterOrder("spaceViewOrderId")

		// then
		require.NoError(t, err)
		assert.Equal(t, "viewOrderId", fx.Details().GetString(bundle.RelationKeySpaceOrder))
		assert.True(t, fx.Details().GetString(bundle.RelationKeySpaceOrder) > "spaceViewOrderId")
	})
}

func TestSpaceView_SetBetweenViews(t *testing.T) {
	t.Run("set view in the beginning", func(t *testing.T) {
		// given
		fx := newSpaceViewFixture(t)
		defer fx.finish()

		// when
		_, err := fx.SetBetweenOrders("", "afterId")

		// then
		require.NoError(t, err)
		assert.NotEmpty(t, fx.Details().GetString(bundle.RelationKeySpaceOrder))
	})
	t.Run("set view between", func(t *testing.T) {
		// given
		fx := newSpaceViewFixture(t)
		defer fx.finish()

		// when
		_, err := fx.SetBetweenOrders("CCCC", "FFFF")

		// then
		require.NoError(t, err)
		orderId := fx.Details().GetString(bundle.RelationKeySpaceOrder)
		require.NotEmpty(t, orderId)
		assert.Greater(t, orderId, "CCCC")
		assert.Greater(t, "FFFF", orderId)
	})
}

func TestSpaceView_SetOrder(t *testing.T) {
	t.Run("set order", func(t *testing.T) {
		// given
		fx := newSpaceViewFixture(t)
		defer fx.finish()

		// when
		prevViewOrderId := ""
		order, err := fx.SetOrder(prevViewOrderId)

		// then
		require.NoError(t, err)
		assert.NotEmpty(t, fx.Details().GetString(bundle.RelationKeySpaceOrder))
		assert.Equal(t, order, fx.Details().GetString(bundle.RelationKeySpaceOrder))
		assert.True(t, fx.Details().GetString(bundle.RelationKeySpaceOrder) > prevViewOrderId)
	})
	t.Run("set order, previous id not empty", func(t *testing.T) {
		// given
		fx := newSpaceViewFixture(t)
		defer fx.finish()

		// when
		prevViewOrderId := "previous"
		order, err := fx.SetOrder(prevViewOrderId)

		// then
		require.NoError(t, err)
		assert.NotEmpty(t, fx.Details().GetString(bundle.RelationKeySpaceOrder))
		assert.Equal(t, order, fx.Details().GetString(bundle.RelationKeySpaceOrder))
		assert.True(t, fx.Details().GetString(bundle.RelationKeySpaceOrder) > prevViewOrderId)
	})
}

type spaceServiceStub struct {
}

func (s *spaceServiceStub) PersonalSpaceId() string {
	return ""
}

func (s *spaceServiceStub) OnViewUpdated(info spaceinfo.SpacePersistentInfo) {
}

func (s *spaceServiceStub) OnWorkspaceChanged(spaceId string, details *domain.Details) {
}

func NewSpaceViewTest(t *testing.T, targetSpaceId string, tree *mock_objecttree.MockObjectTree) (*SpaceView, error) {
	sb := smarttest.NewWithTree("root", tree)
	a := &SpaceView{
		SmartBlock:    sb,
		OrderSettable: order.NewOrderSettable(sb, bundle.RelationKeySpaceOrder),
		spaceService:  &spaceServiceStub{},
		log:           log,
	}

	initCtx := &smartblock.InitContext{
		IsNewObject: true,
	}
	changePayload := &model.ObjectChangePayload{
		Key: targetSpaceId,
	}
	marshaled, err := changePayload.Marshal()
	require.NoError(t, err)
	changeInfo := &treechangeproto.TreeChangeInfo{
		ChangePayload: marshaled,
	}
	tree.EXPECT().ChangeInfo().Return(changeInfo)
	if err := a.Init(initCtx); err != nil {
		return nil, err
	}
	migration.RunMigrations(a, initCtx)
	if err := a.Apply(initCtx.State); err != nil {
		return nil, err
	}
	return a, nil
}

type spaceViewFixture struct {
	*SpaceView
	objectTree *mock_objecttree.MockObjectTree
	ctrl       *gomock.Controller
}

func newSpaceViewFixture(t *testing.T) *spaceViewFixture {
	ctrl := gomock.NewController(t)
	objectTree := mock_objecttree.NewMockObjectTree(ctrl)
	a, err := NewSpaceViewTest(t, "spaceId", objectTree)
	require.NoError(t, err)
	return &spaceViewFixture{
		SpaceView:  a,
		objectTree: objectTree,
		ctrl:       ctrl,
	}
}

func (f *spaceViewFixture) getAccessType() spaceinfo.AccessType {
	return spaceinfo.AccessType(f.CombinedDetails().GetInt64(bundle.RelationKeySpaceAccessType))
}

func (f *spaceViewFixture) finish() {
	f.ctrl.Finish()
}
