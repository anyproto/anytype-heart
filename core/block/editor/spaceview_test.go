package editor

import (
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/acl/list"
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
		err := fx.RefreshAccessType(true)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypePersonal, fx.getAccessType())
		err = fx.SetAclInfo(false, nil, nil, time.Now().Unix())
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypePersonal, fx.getAccessType())
		err = fx.RefreshAccessType(false)
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
		err := fx.RefreshAccessType(false)
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

func TestSpaceView_RefreshAccessType(t *testing.T) {
	t.Run("derives shared from the persisted acl state instead of resetting it", func(t *testing.T) {
		// given a shared space view reloaded with a stale private access type, as left behind when a
		// load skipped ACL processing because the ACL head hadn't moved since the last run
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		fx.seedDetails(t, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyIsAclShared:     domain.Bool(true),
			bundle.RelationKeySpaceAccessType: domain.Int64(spaceinfo.AccessTypePrivate),
		})
		applies := fx.applies()

		// when the space loads
		require.NoError(t, fx.RefreshAccessType(false))

		// then it is shared again, healed with a single apply
		require.Equal(t, spaceinfo.AccessTypeShared, fx.getAccessType())
		require.Equal(t, applies+1, fx.applies())
	})
	t.Run("does not apply when the stored value is already right", func(t *testing.T) {
		// given a space view whose access type matches its persisted ACL state
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		fx.seedDetails(t, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyIsAclShared:     domain.Bool(true),
			bundle.RelationKeySpaceAccessType: domain.Int64(spaceinfo.AccessTypeShared),
		})
		applies := fx.applies()

		// when the space loads, as it does on every app start
		require.NoError(t, fx.RefreshAccessType(false))

		// then nothing is written
		require.Equal(t, spaceinfo.AccessTypeShared, fx.getAccessType())
		require.Equal(t, applies, fx.applies())
	})
	t.Run("writes a missing value once", func(t *testing.T) {
		// given a new space view that has no access type yet
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		require.False(t, fx.LocalDetails().Has(bundle.RelationKeySpaceAccessType))
		applies := fx.applies()

		// when the space loads twice
		require.NoError(t, fx.RefreshAccessType(false))
		require.NoError(t, fx.RefreshAccessType(false))

		// then the value is set explicitly, and only the first load writes it
		require.True(t, fx.LocalDetails().Has(bundle.RelationKeySpaceAccessType))
		require.Equal(t, spaceinfo.AccessTypePrivate, fx.getAccessType())
		require.Equal(t, applies+1, fx.applies())
	})
	t.Run("derives shared from a persisted shareable status alone", func(t *testing.T) {
		// given a shareable space view that no one has joined yet
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		fx.seedDetails(t, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyIsAclShared:          domain.Bool(false),
			bundle.RelationKeySpaceShareableStatus: domain.Int64(spaceinfo.ShareableStatusShareable),
			bundle.RelationKeySpaceAccessType:      domain.Int64(spaceinfo.AccessTypePrivate),
		})

		// when the space loads
		require.NoError(t, fx.RefreshAccessType(false))

		// then it is shared
		require.Equal(t, spaceinfo.AccessTypeShared, fx.getAccessType())
	})
	t.Run("heals a stale shared value to private", func(t *testing.T) {
		// given a space view whose ACL is no longer shared but whose access type still says so
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		fx.seedDetails(t, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyIsAclShared:     domain.Bool(false),
			bundle.RelationKeySpaceAccessType: domain.Int64(spaceinfo.AccessTypeShared),
		})
		applies := fx.applies()

		// when the space loads
		require.NoError(t, fx.RefreshAccessType(false))

		// then it is private, healed with a single apply
		require.Equal(t, spaceinfo.AccessTypePrivate, fx.getAccessType())
		require.Equal(t, applies+1, fx.applies())
	})
	t.Run("leaves a personal space alone", func(t *testing.T) {
		// given the personal space view
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		fx.seedDetails(t, map[domain.RelationKey]domain.Value{
			bundle.RelationKeySpaceAccessType: domain.Int64(spaceinfo.AccessTypePersonal),
		})
		applies := fx.applies()

		// when it loads, whatever the caller thinks it is
		require.NoError(t, fx.RefreshAccessType(true))
		require.NoError(t, fx.RefreshAccessType(false))

		// then it stays personal and nothing is written
		require.Equal(t, spaceinfo.AccessTypePersonal, fx.getAccessType())
		require.Equal(t, applies, fx.applies())
	})
}

func TestSpaceView_SetAclShared(t *testing.T) {
	t.Run("corrects a stale shared state", func(t *testing.T) {
		// given a space view still marked shared after its ACL emptied, because the write of that
		// change failed in a previous session
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		fx.seedDetails(t, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyIsAclShared:     domain.Bool(true),
			bundle.RelationKeySpaceAccessType: domain.Int64(spaceinfo.AccessTypeShared),
		})
		applies := fx.applies()

		// when ACL processing checks it against the loaded ACL
		require.NoError(t, fx.SetAclShared(false))

		// then both the flag and the access type follow the ACL, with a single apply
		require.False(t, fx.LocalDetails().GetBool(bundle.RelationKeyIsAclShared))
		require.Equal(t, spaceinfo.AccessTypePrivate, fx.getAccessType())
		require.Equal(t, applies+1, fx.applies())
	})
	t.Run("does not apply when it matches the acl", func(t *testing.T) {
		// given a space view consistent with its ACL
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		fx.seedDetails(t, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyIsAclShared:     domain.Bool(true),
			bundle.RelationKeySpaceAccessType: domain.Int64(spaceinfo.AccessTypeShared),
		})
		applies := fx.applies()

		// when ACL processing checks it, as it does on every app start
		require.NoError(t, fx.SetAclShared(true))

		// then nothing is written
		require.Equal(t, spaceinfo.AccessTypeShared, fx.getAccessType())
		require.Equal(t, applies, fx.applies())
	})
	t.Run("corrects a stale private state", func(t *testing.T) {
		// given a space view still marked private after members joined, because the write of that
		// change failed in a previous session
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		fx.seedDetails(t, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyIsAclShared:     domain.Bool(false),
			bundle.RelationKeySpaceAccessType: domain.Int64(spaceinfo.AccessTypePrivate),
		})
		applies := fx.applies()

		// when ACL processing checks it against the loaded ACL
		require.NoError(t, fx.SetAclShared(true))

		// then both the flag and the access type follow the ACL, with a single apply
		require.True(t, fx.LocalDetails().GetBool(bundle.RelationKeyIsAclShared))
		require.Equal(t, spaceinfo.AccessTypeShared, fx.getAccessType())
		require.Equal(t, applies+1, fx.applies())
	})
	t.Run("writes a missing flag once", func(t *testing.T) {
		// given a space view that has never stored the flag; false is its zero value, so only
		// presence tells a stored false from a missing one
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		require.False(t, fx.LocalDetails().Has(bundle.RelationKeyIsAclShared))
		applies := fx.applies()

		// when ACL processing checks it on two loads
		require.NoError(t, fx.SetAclShared(false))
		require.NoError(t, fx.SetAclShared(false))

		// then the flag is set explicitly, and only the first load writes it
		require.True(t, fx.LocalDetails().Has(bundle.RelationKeyIsAclShared))
		require.False(t, fx.LocalDetails().GetBool(bundle.RelationKeyIsAclShared))
		require.Equal(t, applies+1, fx.applies())
	})
	t.Run("stays shared through the shareable status when the acl empties", func(t *testing.T) {
		// given a shared space view that is also shareable, e.g. its only member left
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		fx.seedDetails(t, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyIsAclShared:          domain.Bool(true),
			bundle.RelationKeySpaceShareableStatus: domain.Int64(spaceinfo.ShareableStatusShareable),
			bundle.RelationKeySpaceAccessType:      domain.Int64(spaceinfo.AccessTypeShared),
		})

		// when ACL processing finds the ACL empty
		require.NoError(t, fx.SetAclShared(false))

		// then the flag follows the ACL, but the space is still shared: it can be joined by invite
		require.False(t, fx.LocalDetails().GetBool(bundle.RelationKeyIsAclShared))
		require.Equal(t, spaceinfo.AccessTypeShared, fx.getAccessType())
	})
	t.Run("keeps a personal space personal", func(t *testing.T) {
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		require.NoError(t, fx.RefreshAccessType(true))

		require.NoError(t, fx.SetAclShared(true))

		require.Equal(t, spaceinfo.AccessTypePersonal, fx.getAccessType())
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

func TestSpaceView_Info_SkipUnchanged(t *testing.T) {
	t.Run("local info", func(t *testing.T) {
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		info := spaceinfo.NewSpaceLocalInfo("spaceId")
		info.SetLocalStatus(spaceinfo.LocalStatusOk).
			SetRemoteStatus(spaceinfo.RemoteStatusOk).
			SetShareableStatus(spaceinfo.ShareableStatusShareable)
		require.NoError(t, fx.SetSpaceLocalInfo(info))

		// Re-applying the same info is a no-op but must keep the stored values.
		require.NoError(t, fx.SetSpaceLocalInfo(info))
		cur := fx.GetLocalInfo()
		require.Equal(t, spaceinfo.LocalStatusOk, cur.GetLocalStatus())
		require.Equal(t, spaceinfo.RemoteStatusOk, cur.GetRemoteStatus())
		require.Equal(t, spaceinfo.ShareableStatusShareable, cur.GetShareableStatus())

		// A real change still goes through.
		changed := spaceinfo.NewSpaceLocalInfo("spaceId")
		changed.SetLocalStatus(spaceinfo.LocalStatusLoading)
		require.NoError(t, fx.SetSpaceLocalInfo(changed))
		cur = fx.GetLocalInfo()
		require.Equal(t, spaceinfo.LocalStatusLoading, cur.GetLocalStatus())
	})
	t.Run("persistent info", func(t *testing.T) {
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		info := spaceinfo.NewSpacePersistentInfo("spaceId")
		info.SetAccountStatus(spaceinfo.AccountStatusActive)
		require.NoError(t, fx.SetSpacePersistentInfo(info))

		require.NoError(t, fx.SetSpacePersistentInfo(info))
		cur := fx.GetPersistentInfo()
		require.Equal(t, spaceinfo.AccountStatusActive, cur.GetAccountStatus())

		changed := spaceinfo.NewSpacePersistentInfo("spaceId")
		changed.SetAccountStatus(spaceinfo.AccountStatusDeleted)
		require.NoError(t, fx.SetSpacePersistentInfo(changed))
		cur = fx.GetPersistentInfo()
		require.Equal(t, spaceinfo.AccountStatusDeleted, cur.GetAccountStatus())
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

func TestSpaceView_InviteFileInfo(t *testing.T) {
	t.Run("the owner's space view holds the invite itself", func(t *testing.T) {
		// given the owner's space view, which no member of the space syncs
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		want := domain.InviteInfo{
			InviteFileCid: "fileId",
			InviteFileKey: "fileKey",
			InviteType:    domain.InviteTypeAnyone,
			Permissions:   list.AclPermissionsWriter,
			HeldByOwner:   true,
		}

		// when an invite is written to it
		err := fx.SetInviteFileInfo(want)
		require.NoError(t, err)

		// then it reads back whole, and as held by the owner: an invite is only ever put here to be
		// theirs to share
		require.Equal(t, want, fx.GetExistingInviteInfo())

		removed, err := fx.RemoveExistingInviteInfo()
		require.NoError(t, err)
		require.Equal(t, want, removed)
		require.Empty(t, fx.GetExistingInviteInfo())
	})
	t.Run("no invite", func(t *testing.T) {
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		require.Empty(t, fx.GetExistingInviteInfo())
	})
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

// seedDetails stands in for the details a reloaded space view gets back from the store
func (f *spaceViewFixture) seedDetails(t *testing.T, details map[domain.RelationKey]domain.Value) {
	st := f.NewState()
	for key, value := range details {
		st.SetDetailAndBundledRelation(key, value)
	}
	require.NoError(t, f.Apply(st))
}

func (f *spaceViewFixture) applies() int {
	return len(f.SmartBlock.(*smarttest.SmartTest).Results.Applies)
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
