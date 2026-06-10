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
	"github.com/anyproto/anytype-heart/pb"
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

func TestSpaceView_SetPushNotificationMode(t *testing.T) {
	fx := newSpaceViewFixture(t)
	defer fx.finish()
	err := fx.SetPushNotificationMode(nil, pb.RpcPushNotification_Mentions)
	require.NoError(t, err)
	assert.Equal(t, int64(pb.RpcPushNotification_Mentions), fx.Details().GetInt64(bundle.RelationKeySpacePushNotificationMode))
}

func TestSpaceView_SetPushNotificationForceModeIds(t *testing.T) {
	fx := newSpaceViewFixture(t)
	defer fx.finish()
	assert.Error(t, fx.SetPushNotificationForceModeIds(nil, []string{"id1", "id2"}, -1))
	err := fx.SetPushNotificationForceModeIds(nil, []string{"id1", "id2"}, pb.RpcPushNotification_Nothing)
	require.NoError(t, err)
	err = fx.SetPushNotificationForceModeIds(nil, []string{"id2", "id3"}, pb.RpcPushNotification_Mentions)
	require.NoError(t, err)
	err = fx.SetPushNotificationForceModeIds(nil, []string{"id3", "id4"}, pb.RpcPushNotification_All)
	require.NoError(t, err)

	mutedIds := fx.Details().GetStringList(bundle.RelationKeySpacePushNotificationForceMuteIds)
	assert.Equal(t, []string{"id1"}, mutedIds)
	mentionedIds := fx.Details().GetStringList(bundle.RelationKeySpacePushNotificationForceMentionIds)
	assert.Equal(t, []string{"id2"}, mentionedIds)
	allIds := fx.Details().GetStringList(bundle.RelationKeySpacePushNotificationForceAllIds)
	assert.Equal(t, []string{"id3", "id4"}, allIds)
	assert.Equal(t, int64(pb.RpcPushNotification_All), fx.CombinedDetails().GetInt64(bundle.RelationKeySpacePushNotificationMode))
}

func TestSpaceView_ResetPushNotificationIds(t *testing.T) {
	fx := newSpaceViewFixture(t)
	defer fx.finish()
	err := fx.SetPushNotificationForceModeIds(nil, []string{"id1"}, pb.RpcPushNotification_Mentions)
	require.NoError(t, err)
	err = fx.SetPushNotificationForceModeIds(nil, []string{"id2"}, pb.RpcPushNotification_All)
	require.NoError(t, err)
	err = fx.SetPushNotificationForceModeIds(nil, []string{"id3"}, pb.RpcPushNotification_Nothing)
	require.NoError(t, err)
	err = fx.ResetPushNotificationIds(nil, []string{"id1", "id2", "id3"})
	require.NoError(t, err)
	assert.Empty(t, fx.Details().GetStringList(bundle.RelationKeySpacePushNotificationForceMuteIds))
	assert.Empty(t, fx.Details().GetStringList(bundle.RelationKeySpacePushNotificationForceMentionIds))
	assert.Empty(t, fx.Details().GetStringList(bundle.RelationKeySpacePushNotificationForceAllIds))
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

func (s *spaceServiceStub) SpaceViewSetOneToOneIdentity(spaceId string, identity string) {
}

// initSpaceViewTest is the single shared construction path for SpaceView tests: it builds the
// smarttest tree + SpaceView, optionally seeds persisted local/remote status (simulating a reload
// from disk), then runs Init / migrations / Apply. Both NewSpaceViewTest (brand-new object) and
// buildSpaceViewWithSeededLocalInfo (reload-from-disk) delegate here so the construction lives in
// one place.
func initSpaceViewTest(t *testing.T, targetSpaceId string, tree *mock_objecttree.MockObjectTree, isNewObject bool, seeded *spaceinfo.SpaceLocalInfo) (*SpaceView, error) {
	sb := smarttest.NewWithTree("root", tree)
	a := &SpaceView{
		SmartBlock:    sb,
		OrderSettable: order.NewOrderSettable(sb, bundle.RelationKeySpaceOrder),
		spaceService:  &spaceServiceStub{},
		log:           log,
	}

	changePayload := &model.ObjectChangePayload{
		Key: targetSpaceId,
	}
	marshaled, err := changePayload.Marshal()
	require.NoError(t, err)
	changeInfo := &treechangeproto.TreeChangeInfo{
		ChangePayload: marshaled,
	}
	tree.EXPECT().ChangeInfo().Return(changeInfo).AnyTimes()

	initCtx := &smartblock.InitContext{
		IsNewObject: isNewObject,
	}
	if seeded != nil {
		seedState := sb.NewState()
		seedState.SetLocalDetail(bundle.RelationKeySpaceLocalStatus, domain.Int64(int64(seeded.GetLocalStatus())))
		seedState.SetLocalDetail(bundle.RelationKeySpaceRemoteStatus, domain.Int64(int64(seeded.GetRemoteStatus())))
		initCtx.State = seedState

		// NOTE: this only verifies our own hand-seeded state, NOT the real cross-layer carry. In
		// production the persisted spaceLocalStatus reaches ctx.State via
		// smartblock.Init->injectLocalDetails, which copies it because spaceLocalStatus is a
		// source=derived relation (see relations.json / bundle.LocalAndDerivedRelationKeys). The
		// smarttest harness has no object store and does not run injectLocalDetails, so that carry
		// is *assumed here* (verified once, manually) rather than exercised by this test. The
		// assertion below just pins our setup precondition so the post-Init assertion is meaningful.
		preInfo := spaceinfo.NewSpaceLocalInfoFromState(initCtx.State)
		require.Equal(t, seeded.GetLocalStatus(), preInfo.GetLocalStatus(),
			"seeded localStatus must be present in ctx.State going into Init (setup precondition)")
	}

	if err := a.Init(initCtx); err != nil {
		return nil, err
	}
	migration.RunMigrations(a, initCtx)
	if err := a.Apply(initCtx.State); err != nil {
		return nil, err
	}
	return a, nil
}

func NewSpaceViewTest(t *testing.T, targetSpaceId string, tree *mock_objecttree.MockObjectTree) (*SpaceView, error) {
	return initSpaceViewTest(t, targetSpaceId, tree, true, nil)
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

// buildSpaceViewWithSeededLocalInfo constructs a SpaceView whose underlying doc already has
// localStatus/remoteStatus persisted (simulating a reload from the object store after a
// previous session), then runs Init exactly as the production reload path does. It delegates to
// initSpaceViewTest so SpaceView construction stays defined in one place.
func buildSpaceViewWithSeededLocalInfo(t *testing.T, targetSpaceId string, seeded *spaceinfo.SpaceLocalInfo) *SpaceView {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	tree := mock_objecttree.NewMockObjectTree(ctrl)

	sv, err := initSpaceViewTest(t, targetSpaceId, tree, false, seeded)
	require.NoError(t, err)
	return sv
}

func TestSpaceView_Init_PreservesPersistedLocalStatus(t *testing.T) {
	t.Run("previously Ok is preserved", func(t *testing.T) {
		// given
		seeded := spaceinfo.NewSpaceLocalInfo("spaceId")
		seeded.SetLocalStatus(spaceinfo.LocalStatusOk).
			SetRemoteStatus(spaceinfo.RemoteStatusOk)

		// when
		sv := buildSpaceViewWithSeededLocalInfo(t, "spaceId", &seeded)

		// then
		got := sv.GetLocalInfo()
		assert.Equal(t, spaceinfo.LocalStatusOk, got.GetLocalStatus())
		assert.Equal(t, spaceinfo.RemoteStatusOk, got.GetRemoteStatus())
	})

	t.Run("brand new spaceview defaults to Unknown", func(t *testing.T) {
		// given no seeded local info
		// when
		sv := buildSpaceViewWithSeededLocalInfo(t, "spaceId", nil)

		// then
		got := sv.GetLocalInfo()
		assert.Equal(t, spaceinfo.LocalStatusUnknown, got.GetLocalStatus())
		assert.Equal(t, spaceinfo.RemoteStatusUnknown, got.GetRemoteStatus())
	})
}
