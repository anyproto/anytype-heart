package aclobjectmanager

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anyproto/any-sync/accountservice/mock_accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/mock_commonspace"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/acl/recordverifier"
	"github.com/anyproto/any-sync/commonspace/object/acl/syncacl"
	"github.com/anyproto/any-sync/commonspace/object/acl/syncacl/headupdater"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/commonspace/sync/syncdeps"
	"github.com/anyproto/any-sync/commonspace/syncstatus"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"
	"storj.io/drpc"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/aclnotifications/mock_aclnotifications"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies/mock_dependencies"
	"github.com/anyproto/anytype-heart/space/internal/components/participantwatcher/mock_participantwatcher"
	"github.com/anyproto/anytype-heart/space/internal/components/spaceloader/mock_spaceloader"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus/mock_spacestatus"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

func TestAclObjectManager(t *testing.T) {
	t.Run("owner", func(t *testing.T) {
		a := list.NewAclExecutor("spaceId")
		cmds := []string{
			"a.init::a",
		}
		for _, cmd := range cmds {
			err := a.Execute(cmd)
			require.NoError(t, err)
		}
		acl := &syncAclStub{AclList: a.ActualAccounts()["a"].Acl}
		fx := newFixture(t)
		defer fx.finish(t)
		fx.mockLoader.EXPECT().WaitLoad(mock.Anything).Return(fx.mockSpace, nil)
		fx.mockSpace.EXPECT().CommonSpace().Return(fx.mockCommonSpace)
		fx.mockSpace.EXPECT().Id().Return("spaceId")
		fx.mockSpace.EXPECT().IsOneToOne().Return(false).Maybe()
		fx.mockCommonSpace.EXPECT().Acl().AnyTimes().Return(acl)
		fx.mockStatus.EXPECT().GetLatestAclHeadId().Return("")
		fx.mockParticipantWatcher.EXPECT().GetProcessedAclHeadId(mock.Anything, fx.mockSpace).Return("").Once()
		fx.mockParticipantWatcher.EXPECT().SetProcessedAclHeadId(mock.Anything, fx.mockSpace, acl.Head().Id).Return(nil).Once()
		fx.mockStatus.EXPECT().SetOwner(acl.AclState().Identity().Account(), mock.Anything).Return(nil)
		fx.mockParticipantWatcher.EXPECT().UpdateParticipantFromAclState(mock.Anything, fx.mockSpace, mock.Anything).
			RunAndReturn(func(_ context.Context, space clientspace.Space, state list.AccountState) error {
				require.True(t, state.PubKey.Equals(acl.AclState().Identity()))
				return nil
			})
		fx.mockParticipantWatcher.EXPECT().WatchParticipant(mock.Anything, fx.mockSpace, mock.Anything, mock.Anything).Return(nil)
		fx.mockStatus.EXPECT().SetAclInfo(true, nil, nil, mock.Anything).Return(nil)
		fx.mockStatus.EXPECT().SetMyParticipantStatus(mock.Anything).Return(nil)
		fx.mockCommonSpace.EXPECT().Id().AnyTimes().Return("spaceId")
		fx.mockStatus.EXPECT().GetLocalStatus().Return(spaceinfo.LocalStatusOk)
		fx.mockAclNotification.EXPECT().AddRecords(acl, list.AclPermissionsOwner, "spaceId", spaceinfo.AccountStatusActive, spaceinfo.LocalStatusOk)
		fx.run(t)
		<-fx.aclObjectManager.wait
		fx.aclObjectManager.mx.Lock()
		defer fx.aclObjectManager.mx.Unlock()
		require.Equal(t, acl.Head().Id, fx.aclObjectManager.lastIndexed)
		require.Equal(t, fx.aclObjectManager, acl.updater)
		require.Equal(t, "spaceId", fx.spaceLoaderListener.space)
	})
	t.Run("participant", func(t *testing.T) {
		a := list.NewAclExecutor("spaceId")
		cmds := []string{
			"a.init::a",
			"a.invite::invId",
			"b.join::invId",
			"a.approve::b,r",
		}
		for _, cmd := range cmds {
			err := a.Execute(cmd)
			require.NoError(t, err)
		}
		acl := &syncAclStub{AclList: a.ActualAccounts()["b"].Acl}
		fx := newFixture(t)
		defer fx.finish(t)
		fx.mockLoader.EXPECT().WaitLoad(mock.Anything).Return(fx.mockSpace, nil)
		fx.mockSpace.EXPECT().CommonSpace().Return(fx.mockCommonSpace)
		fx.mockSpace.EXPECT().Id().Return("spaceId")
		fx.mockSpace.EXPECT().IsOneToOne().Return(false).Maybe()
		fx.mockCommonSpace.EXPECT().Acl().AnyTimes().Return(acl)
		fx.mockStatus.EXPECT().SetOwner(a.ActualAccounts()["a"].Acl.AclState().Identity().Account(), mock.Anything).Return(nil)
		fx.mockStatus.EXPECT().GetLatestAclHeadId().Return("")
		fx.mockParticipantWatcher.EXPECT().GetProcessedAclHeadId(mock.Anything, fx.mockSpace).Return("").Once()
		fx.mockParticipantWatcher.EXPECT().SetProcessedAclHeadId(mock.Anything, fx.mockSpace, acl.Head().Id).Return(nil).Once()
		var callCounter atomic.Bool
		fx.mockParticipantWatcher.EXPECT().UpdateParticipantFromAclState(mock.Anything, fx.mockSpace, mock.Anything).
			RunAndReturn(func(_ context.Context, space clientspace.Space, state list.AccountState) error {
				if !callCounter.Load() {
					require.True(t, state.PubKey.Equals(a.ActualAccounts()["a"].Keys.SignKey.GetPublic()))
					callCounter.Store(true)
				} else {
					require.True(t, state.PubKey.Equals(acl.AclState().Identity()))
				}
				return nil
			})
		fx.mockParticipantWatcher.EXPECT().WatchParticipant(mock.Anything, fx.mockSpace, mock.Anything, mock.Anything).Return(nil)
		fx.mockStatus.EXPECT().SetAclInfo(false, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		fx.mockStatus.EXPECT().SetMyParticipantStatus(mock.Anything).Return(nil)
		fx.mockCommonSpace.EXPECT().Id().AnyTimes().Return("spaceId")
		fx.mockStatus.EXPECT().GetLocalStatus().Return(spaceinfo.LocalStatusOk)
		fx.mockAclNotification.EXPECT().AddRecords(acl, list.AclPermissionsReader, "spaceId", spaceinfo.AccountStatusActive, spaceinfo.LocalStatusOk)
		fx.run(t)
		<-fx.aclObjectManager.wait
		fx.aclObjectManager.mx.Lock()
		defer fx.aclObjectManager.mx.Unlock()
		require.Equal(t, acl.Head().Id, fx.aclObjectManager.lastIndexed)
		require.Equal(t, fx.aclObjectManager, acl.updater)
	})
	t.Run("participant removed", func(t *testing.T) {
		a := list.NewAclExecutor("spaceId")
		cmds := []string{
			"a.init::a",
			"a.invite::invId",
			"b.join::invId",
			"a.approve::b,r",
			"a.remove::b",
		}
		for _, cmd := range cmds {
			err := a.Execute(cmd)
			require.NoError(t, err)
		}
		acl := &syncAclStub{AclList: a.ActualAccounts()["b"].Acl}
		fx := newFixture(t)
		defer fx.finish(t)
		fx.mockLoader.EXPECT().WaitLoad(mock.Anything).Return(fx.mockSpace, nil)
		fx.mockStatus.EXPECT().SetOwner(a.ActualAccounts()["a"].Acl.AclState().Identity().Account(), mock.Anything).Return(nil)
		fx.mockSpace.EXPECT().CommonSpace().Return(fx.mockCommonSpace)
		fx.mockSpace.EXPECT().Id().Return("spaceId")
		fx.mockSpace.EXPECT().IsOneToOne().Return(false).Maybe()
		fx.mockCommonSpace.EXPECT().Acl().AnyTimes().Return(acl)
		fx.mockStatus.EXPECT().GetLatestAclHeadId().Return("")
		fx.mockParticipantWatcher.EXPECT().GetProcessedAclHeadId(mock.Anything, fx.mockSpace).Return("").Once()
		fx.mockParticipantWatcher.EXPECT().SetProcessedAclHeadId(mock.Anything, fx.mockSpace, acl.Head().Id).Return(nil).Once()
		fx.mockParticipantWatcher.EXPECT().UpdateParticipantFromAclState(mock.Anything, fx.mockSpace, mock.Anything).
			RunAndReturn(func(_ context.Context, space clientspace.Space, state list.AccountState) error {
				require.True(t, state.PubKey.Equals(a.ActualAccounts()["a"].Keys.SignKey.GetPublic()))
				return nil
			})
		fx.mockParticipantWatcher.EXPECT().WatchParticipant(mock.Anything, fx.mockSpace, mock.Anything, mock.Anything).Return(nil)
		fx.mockStatus.EXPECT().SetAclInfo(false, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		fx.mockStatus.EXPECT().SetMyParticipantStatus(model.ParticipantStatus_Removed).Return(nil)
		fx.mockStatus.EXPECT().SetPersistentStatus(spaceinfo.AccountStatusDeleted).Return(nil)
		fx.mockCommonSpace.EXPECT().Id().AnyTimes().Return("spaceId")
		fx.mockStatus.EXPECT().GetLocalStatus().Return(spaceinfo.LocalStatusOk)
		fx.mockAclNotification.EXPECT().AddRecords(acl, list.AclPermissionsNone, "spaceId", spaceinfo.AccountStatusDeleted, spaceinfo.LocalStatusOk)
		fx.run(t)
		<-fx.aclObjectManager.wait
		fx.aclObjectManager.mx.Lock()
		defer fx.aclObjectManager.mx.Unlock()
		require.Equal(t, acl.Head().Id, fx.aclObjectManager.lastIndexed)
		require.Equal(t, fx.aclObjectManager, acl.updater)
	})
	// b is removed while offline, which rotates the read key, and only then does c ask to join. c's
	// metadata is keyed to the new read key, which b never got
	removedThenJoined := []string{
		"a.init::a",
		"a.invite::invId",
		"b.join::invId",
		"a.approve::b,r",
		"a.remove::b",
		"a.invite::invId2",
		"c.join::invId2",
	}
	newMemberAcl := func(t *testing.T, cmds []string, member string) (*list.AclTestExecutor, *syncAclStub) {
		a := list.NewAclExecutor("spaceId")
		for _, cmd := range cmds {
			require.NoError(t, a.Execute(cmd))
		}
		return a, &syncAclStub{AclList: a.ActualAccounts()[member].Acl}
	}
	for _, tc := range []struct {
		name string
		cmds []string
		// whether the space view knows the acl head this device has, instead of no head at all
		latestAclHeadIsHead bool
	}{
		{
			name: "participant removed while offline, then a newcomer asked to join",
			cmds: removedThenJoined,
		},
		{
			name:                "participant removed while offline, then a newcomer was accepted",
			cmds:                append(slices.Clone(removedThenJoined), "a.approve::c,r"),
			latestAclHeadIsHead: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			a, acl := newMemberAcl(t, tc.cmds, "b")
			_, err := acl.AclState().GetMetadata(a.ActualAccounts()["c"].Keys.SignKey.GetPublic(), true)
			require.ErrorIs(t, err, list.ErrFailedToDecrypt, "the removed member must not be able to read the newcomer's metadata")
			fx := newFixture(t)
			defer fx.finish(t)
			fx.mockLoader.EXPECT().WaitLoad(mock.Anything).Return(fx.mockSpace, nil)
			fx.mockSpace.EXPECT().CommonSpace().Return(fx.mockCommonSpace)
			fx.mockSpace.EXPECT().Id().Return("spaceId")
			fx.mockCommonSpace.EXPECT().Acl().AnyTimes().Return(acl)
			fx.mockCommonSpace.EXPECT().Id().AnyTimes().Return("spaceId")
			latestAclHeadId := ""
			if tc.latestAclHeadIsHead {
				latestAclHeadId = acl.Head().Id
			}
			fx.mockStatus.EXPECT().GetLatestAclHeadId().Return(latestAclHeadId).Once()
			fx.mockParticipantWatcher.EXPECT().GetProcessedAclHeadId(mock.Anything, fx.mockSpace).Return("").Once()
			fx.mockStatus.EXPECT().GetLocalStatus().Return(spaceinfo.LocalStatusOk)

			// then: the space is dropped although the other members cannot be processed; the strict
			// mocks verify that nobody else's participant is touched
			fx.mockStatus.EXPECT().SetMyParticipantStatus(model.ParticipantStatus_Removed).Return(nil).Once()
			fx.mockStatus.EXPECT().SetPersistentStatus(spaceinfo.AccountStatusDeleted).Return(nil).Once()
			fx.mockAclNotification.EXPECT().AddRecords(acl, list.AclPermissionsNone, "spaceId", spaceinfo.AccountStatusDeleted, spaceinfo.LocalStatusOk).Once()

			// when
			fx.run(t)
			<-fx.aclObjectManager.wait
		})
	}
	// in each case below some member's metadata cannot be decrypted, and our own account must not be
	// taken for removed: the space is kept and the decryption error is returned. The strict mocks
	// verify that neither participant status nor account status is written, and no AddRecords call
	// either, as processing fails
	for _, tc := range []struct {
		name            string
		acl             func(t *testing.T) (*syncAclStub, crypto.PrivKey)
		latestAclHeadId string
	}{
		{
			name: "removed participant keeps the space until its acl reaches the latest known head",
			acl: func(t *testing.T) (*syncAclStub, crypto.PrivKey) {
				_, acl := newMemberAcl(t, removedThenJoined, "b")
				return acl, nil
			},
			// the space view knows an acl head this device has not received yet
			latestAclHeadId: "notReceivedHeadId",
		},
		{
			name: "guest is not marked removed",
			acl: func(t *testing.T) (*syncAclStub, crypto.PrivKey) {
				// processStates never matches a guest's own account, so neither may the fallback
				a, acl := newMemberAcl(t, removedThenJoined, "b")
				return acl, a.ActualAccounts()["b"].Keys.SignKey
			},
		},
		{
			name: "removed participant who asked to join again is left to the join flow",
			acl: func(t *testing.T) (*syncAclStub, crypto.PrivKey) {
				// b's own request is keyed to the new read key: b is joining, with no permissions
				_, acl := newMemberAcl(t, append(slices.Clone(removedThenJoined), "b.join::invId2"), "b")
				return acl, nil
			},
		},
		{
			name: "account absent from the acl is not marked removed",
			acl: func(t *testing.T) (*syncAclStub, crypto.PrivKey) {
				// another member, b, was removed, and an outsider holds none of the keys, so not even
				// the owner's metadata decrypts
				a, _ := newMemberAcl(t, removedThenJoined, "a")
				ownerAcl := a.ActualAccounts()["a"].Acl
				records, err := ownerAcl.RecordsAfter(ctx, "")
				require.NoError(t, err)
				storage, err := list.NewInMemoryStorage(ownerAcl.Id(), records)
				require.NoError(t, err)
				outsiderKeys, err := accountdata.NewRandom()
				require.NoError(t, err)
				outsiderAcl, err := list.BuildAclListWithIdentity(outsiderKeys, storage, recordverifier.NewValidateFull())
				require.NoError(t, err)
				return &syncAclStub{AclList: outsiderAcl}, nil
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			acl, guestKey := tc.acl(t)
			fx := newFixture(t)
			defer fx.finish(t)
			fx.aclObjectManager.guestKey = guestKey
			fx.mockLoader.EXPECT().WaitLoad(mock.Anything).Return(fx.mockSpace, nil)
			fx.mockSpace.EXPECT().CommonSpace().Return(fx.mockCommonSpace)
			fx.mockSpace.EXPECT().Id().Return("spaceId")
			fx.mockCommonSpace.EXPECT().Acl().AnyTimes().Return(acl)
			fx.mockCommonSpace.EXPECT().Id().AnyTimes().Return("spaceId")
			// once at startup, once more when the test re-runs processing to see its error
			fx.mockStatus.EXPECT().GetLatestAclHeadId().Return(tc.latestAclHeadId).Times(2)
			fx.mockParticipantWatcher.EXPECT().GetProcessedAclHeadId(mock.Anything, fx.mockSpace).Return("").Times(2)
			fx.run(t)
			<-fx.aclObjectManager.wait

			// when
			err := fx.reprocessAcl(acl)

			// then
			require.ErrorIs(t, err, list.ErrFailedToDecrypt)
		})
	}
	t.Run("skip member processing when persisted acl head matches", func(t *testing.T) {
		a := list.NewAclExecutor("spaceId")
		cmds := []string{
			"a.init::a",
			"a.invite::invId",
			"b.join::invId",
			"a.approve::b,r",
		}
		for _, cmd := range cmds {
			err := a.Execute(cmd)
			require.NoError(t, err)
		}
		acl := &syncAclStub{AclList: a.ActualAccounts()["a"].Acl}
		fx := newFixture(t)
		defer fx.finish(t)
		fx.mockLoader.EXPECT().WaitLoad(mock.Anything).Return(fx.mockSpace, nil)
		fx.mockSpace.EXPECT().CommonSpace().Return(fx.mockCommonSpace)
		fx.mockSpace.EXPECT().Id().Return("spaceId")
		fx.mockCommonSpace.EXPECT().Acl().AnyTimes().Return(acl)
		fx.mockCommonSpace.EXPECT().Id().AnyTimes().Return("spaceId")
		fx.mockStatus.EXPECT().GetLatestAclHeadId().Return("")
		fx.mockParticipantWatcher.EXPECT().GetProcessedAclHeadId(mock.Anything, fx.mockSpace).Return(acl.Head().Id).Once()
		fx.mockParticipantWatcher.EXPECT().WatchPersistedParticipants(mock.Anything, fx.mockSpace).Return(nil).Once()
		fx.mockStatus.EXPECT().SetAclShared(true).Return(nil).Once()
		fx.mockStatus.EXPECT().GetLocalStatus().Return(spaceinfo.LocalStatusOk)
		fx.mockAclNotification.EXPECT().AddRecords(acl, mock.Anything, "spaceId", spaceinfo.AccountStatusActive, spaceinfo.LocalStatusOk)
		// no UpdateParticipantFromAclState / WatchParticipant / SetOwner / SetAclInfo
		// expectations: the strict mocks verify the per-member path is not entered
		fx.run(t)
		<-fx.aclObjectManager.wait
		fx.aclObjectManager.mx.Lock()
		defer fx.aclObjectManager.mx.Unlock()
		require.Equal(t, acl.Head().Id, fx.aclObjectManager.lastIndexed)
		require.Equal(t, fx.aclObjectManager, acl.updater)
	})
	t.Run("check acl shared against an emptied acl when persisted acl head matches", func(t *testing.T) {
		// an owner-only ACL: a stale isAclShared persisted by a previous session must be corrected
		// even though the per-member path is skipped
		a := list.NewAclExecutor("spaceId")
		require.NoError(t, a.Execute("a.init::a"))
		acl := &syncAclStub{AclList: a.ActualAccounts()["a"].Acl}
		fx := newFixture(t)
		defer fx.finish(t)
		fx.mockLoader.EXPECT().WaitLoad(mock.Anything).Return(fx.mockSpace, nil)
		fx.mockSpace.EXPECT().CommonSpace().Return(fx.mockCommonSpace)
		fx.mockSpace.EXPECT().Id().Return("spaceId")
		fx.mockCommonSpace.EXPECT().Acl().AnyTimes().Return(acl)
		fx.mockCommonSpace.EXPECT().Id().AnyTimes().Return("spaceId")
		fx.mockStatus.EXPECT().GetLatestAclHeadId().Return("")
		fx.mockParticipantWatcher.EXPECT().GetProcessedAclHeadId(mock.Anything, fx.mockSpace).Return(acl.Head().Id).Once()
		fx.mockParticipantWatcher.EXPECT().WatchPersistedParticipants(mock.Anything, fx.mockSpace).Return(nil).Once()
		fx.mockStatus.EXPECT().SetAclShared(false).Return(nil).Once()
		fx.mockStatus.EXPECT().GetLocalStatus().Return(spaceinfo.LocalStatusOk)
		fx.mockAclNotification.EXPECT().AddRecords(acl, list.AclPermissionsOwner, "spaceId", spaceinfo.AccountStatusActive, spaceinfo.LocalStatusOk)
		fx.run(t)
		<-fx.aclObjectManager.wait
		fx.aclObjectManager.mx.Lock()
		defer fx.aclObjectManager.mx.Unlock()
		require.Equal(t, acl.Head().Id, fx.aclObjectManager.lastIndexed)
	})
	t.Run("finish loading when the acl shared check fails on the skip path", func(t *testing.T) {
		// the check only repairs a stale flag: failing to write it must not fail the space load
		// or re-enter the per-member path
		a := list.NewAclExecutor("spaceId")
		require.NoError(t, a.Execute("a.init::a"))
		acl := &syncAclStub{AclList: a.ActualAccounts()["a"].Acl}
		fx := newFixture(t)
		defer fx.finish(t)
		fx.mockLoader.EXPECT().WaitLoad(mock.Anything).Return(fx.mockSpace, nil)
		fx.mockSpace.EXPECT().CommonSpace().Return(fx.mockCommonSpace)
		fx.mockSpace.EXPECT().Id().Return("spaceId")
		fx.mockCommonSpace.EXPECT().Acl().AnyTimes().Return(acl)
		fx.mockCommonSpace.EXPECT().Id().AnyTimes().Return("spaceId")
		fx.mockStatus.EXPECT().GetLatestAclHeadId().Return("")
		fx.mockParticipantWatcher.EXPECT().GetProcessedAclHeadId(mock.Anything, fx.mockSpace).Return(acl.Head().Id).Once()
		fx.mockParticipantWatcher.EXPECT().WatchPersistedParticipants(mock.Anything, fx.mockSpace).Return(nil).Once()
		fx.mockStatus.EXPECT().SetAclShared(false).Return(errors.New("apply failed")).Once()
		fx.mockStatus.EXPECT().GetLocalStatus().Return(spaceinfo.LocalStatusOk)
		fx.mockAclNotification.EXPECT().AddRecords(acl, list.AclPermissionsOwner, "spaceId", spaceinfo.AccountStatusActive, spaceinfo.LocalStatusOk)
		fx.run(t)
		<-fx.aclObjectManager.wait
		fx.aclObjectManager.mx.Lock()
		defer fx.aclObjectManager.mx.Unlock()
		require.Equal(t, acl.Head().Id, fx.aclObjectManager.lastIndexed)
	})
	t.Run("fall back to full processing when persisted watch fails", func(t *testing.T) {
		a := list.NewAclExecutor("spaceId")
		cmds := []string{
			"a.init::a",
		}
		for _, cmd := range cmds {
			err := a.Execute(cmd)
			require.NoError(t, err)
		}
		acl := &syncAclStub{AclList: a.ActualAccounts()["a"].Acl}
		fx := newFixture(t)
		defer fx.finish(t)
		fx.mockLoader.EXPECT().WaitLoad(mock.Anything).Return(fx.mockSpace, nil)
		fx.mockSpace.EXPECT().CommonSpace().Return(fx.mockCommonSpace)
		fx.mockSpace.EXPECT().Id().Return("spaceId")
		fx.mockSpace.EXPECT().IsOneToOne().Return(false).Maybe()
		fx.mockCommonSpace.EXPECT().Acl().AnyTimes().Return(acl)
		fx.mockCommonSpace.EXPECT().Id().AnyTimes().Return("spaceId")
		fx.mockStatus.EXPECT().GetLatestAclHeadId().Return("").Times(2)
		fx.mockParticipantWatcher.EXPECT().GetProcessedAclHeadId(mock.Anything, fx.mockSpace).Return(acl.Head().Id).Once()
		fx.mockParticipantWatcher.EXPECT().WatchPersistedParticipants(mock.Anything, fx.mockSpace).Return(context.DeadlineExceeded).Once()
		fx.mockParticipantWatcher.EXPECT().SetProcessedAclHeadId(mock.Anything, fx.mockSpace, acl.Head().Id).Return(nil).Once()
		fx.mockStatus.EXPECT().SetOwner(acl.AclState().Identity().Account(), mock.Anything).Return(nil)
		fx.mockParticipantWatcher.EXPECT().UpdateParticipantFromAclState(mock.Anything, fx.mockSpace, mock.Anything).Return(nil)
		fx.mockParticipantWatcher.EXPECT().WatchParticipant(mock.Anything, fx.mockSpace, mock.Anything, mock.Anything).Return(nil)
		fx.mockStatus.EXPECT().SetAclInfo(true, nil, nil, mock.Anything).Return(nil)
		fx.mockStatus.EXPECT().SetMyParticipantStatus(mock.Anything).Return(nil)
		fx.mockStatus.EXPECT().GetLocalStatus().Return(spaceinfo.LocalStatusOk)
		fx.mockAclNotification.EXPECT().AddRecords(acl, list.AclPermissionsOwner, "spaceId", spaceinfo.AccountStatusActive, spaceinfo.LocalStatusOk)
		fx.run(t)
		<-fx.aclObjectManager.wait
		fx.aclObjectManager.mx.Lock()
		defer fx.aclObjectManager.mx.Unlock()
		require.Equal(t, acl.Head().Id, fx.aclObjectManager.lastIndexed)
	})
}

var ctx = context.Background()

type fixture struct {
	*aclObjectManager
	a                      *app.App
	ctrl                   *gomock.Controller
	mockStatus             *mock_spacestatus.MockSpaceStatus
	mockIndexer            *mock_dependencies.MockSpaceIndexer
	mockLoader             *mock_spaceloader.MockSpaceLoader
	mockSpace              *mock_clientspace.MockSpace
	mockCommonSpace        *mock_commonspace.MockSpace
	mockParticipantWatcher *mock_participantwatcher.MockParticipantWatcher
	mockAclNotification    *mock_aclnotifications.MockAclNotification
	mockAccountService     *mock_accountservice.MockService
	spaceLoaderListener    *testSpaceLoaderListener
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	fx := &fixture{
		aclObjectManager:       New(nil, nil).(*aclObjectManager),
		ctrl:                   ctrl,
		a:                      new(app.App),
		mockStatus:             mock_spacestatus.NewMockSpaceStatus(t),
		mockIndexer:            mock_dependencies.NewMockSpaceIndexer(t),
		mockLoader:             mock_spaceloader.NewMockSpaceLoader(t),
		mockSpace:              mock_clientspace.NewMockSpace(t),
		mockCommonSpace:        mock_commonspace.NewMockSpace(ctrl),
		mockParticipantWatcher: mock_participantwatcher.NewMockParticipantWatcher(t),
		mockAclNotification:    mock_aclnotifications.NewMockAclNotification(t),
		mockAccountService:     mock_accountservice.NewMockService(ctrl),
		spaceLoaderListener:    &testSpaceLoaderListener{},
	}

	accKeys, err := accountdata.NewRandom()
	require.NoError(t, err)

	fx.mockAccountService.EXPECT().Account().Return(accKeys).AnyTimes()
	fx.a.Register(testutil.PrepareMock(ctx, fx.a, fx.mockStatus)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockIndexer)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockLoader)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockParticipantWatcher)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockAclNotification)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockAccountService)).
		Register(fx.spaceLoaderListener).
		Register(fx)
	return fx
}

func (fx *fixture) run(t *testing.T) {
	require.NoError(t, fx.a.Start(ctx))
}

func (fx *fixture) finish(t *testing.T) {
	require.NoError(t, fx.a.Close(ctx))
	fx.ctrl.Finish()
}

// reprocessAcl runs ACL processing again the way process() does at startup, and returns the error
// that process() only logs
func (fx *fixture) reprocessAcl(acl *syncAclStub) error {
	acl.RLock()
	defer acl.RUnlock()
	return fx.aclObjectManager.processAcl()
}

type syncAclStub struct {
	list.AclList
	updater headupdater.AclUpdater
}

var _ syncacl.SyncAcl = &syncAclStub{}

func (s *syncAclStub) HandleMessage(ctx context.Context, senderId string, protoVersion uint32, message *spacesyncproto.ObjectSyncMessage) (err error) {
	return
}

func (s *syncAclStub) SyncWithPeer(ctx context.Context, p peer.Peer) (err error) {
	return
}

func (s *syncAclStub) Init(a *app.App) (err error) {
	return nil
}

func (s *syncAclStub) Name() (name string) {
	return syncacl.CName
}

func (s *syncAclStub) Run(ctx context.Context) (err error) {
	return
}

func (s *syncAclStub) HandleRequest(ctx context.Context, senderId string, request *spacesyncproto.ObjectSyncMessage) (response *spacesyncproto.ObjectSyncMessage, err error) {
	return
}

func (s *syncAclStub) SetAclUpdater(updater headupdater.AclUpdater) {
	s.updater = updater
	return
}

func (s *syncAclStub) HandleHeadUpdate(ctx context.Context, statusUpdater syncstatus.StatusUpdater, headUpdate drpc.Message) (syncdeps.Request, error) {
	return nil, nil
}
func (s *syncAclStub) HandleStreamRequest(ctx context.Context, rq syncdeps.Request, updater syncdeps.QueueSizeUpdater, send func(resp proto.Message) error) (syncdeps.Request, error) {
	return nil, nil
}
func (s *syncAclStub) HandleDeprecatedRequest(ctx context.Context, req *spacesyncproto.ObjectSyncMessage) (resp *spacesyncproto.ObjectSyncMessage, err error) {
	return nil, nil
}

func (s *syncAclStub) HandleResponse(ctx context.Context, peerId string, objectId string, resp syncdeps.Response) error {
	return nil
}
func (s *syncAclStub) ResponseCollector() syncdeps.ResponseCollector {
	return nil
}

type testSpaceLoaderListener struct {
	SpaceLoaderListener
	app.Component
	space string
}

func (s *testSpaceLoaderListener) OnSpaceLoad(space string) {
	s.space = space
}
func (s *testSpaceLoaderListener) OnSpaceUnload(_ string) {}

func (s *testSpaceLoaderListener) Init(a *app.App) (err error) { return nil }
func (s *testSpaceLoaderListener) Name() (name string)         { return "spaceLoaderListener" }

// TestAclObjectManagerUpdateAclUnderAclWriteLock covers GO-7525: any-sync invokes the
// UpdateAcl callback while holding the ACL write lock (syncacl.syncAclHandler.HandleResponse
// locks before AddRawRecords), so nothing reachable from the callback may take the ACL read
// lock again. sync.RWMutex is not reentrant: the writer never returns, and every reader in
// that space - tree building, change validation, object restrictions - stalls behind it for
// good, which is what makes ObjectOpen hang forever.
func TestAclObjectManagerUpdateAclUnderAclWriteLock(t *testing.T) {
	a := list.NewAclExecutor("spaceId")
	for _, cmd := range []string{"a.init::a", "a.invite::invId", "b.join::invId", "a.approve::b,r"} {
		require.NoError(t, a.Execute(cmd))
	}
	acl := &syncAclStub{AclList: a.ActualAccounts()["a"].Acl}

	fx := newFixture(t)
	defer fx.finish(t)
	fx.mockLoader.EXPECT().WaitLoad(mock.Anything).Return(fx.mockSpace, nil)
	fx.mockSpace.EXPECT().CommonSpace().Return(fx.mockCommonSpace)
	fx.mockSpace.EXPECT().Id().Return("spaceId")
	fx.mockCommonSpace.EXPECT().Acl().AnyTimes().Return(acl)
	fx.mockCommonSpace.EXPECT().Id().AnyTimes().Return("spaceId")
	// Both doubles read the ACL exactly the way their clientspace.(*space) counterparts do, so
	// this test deadlocks if the manager itself starts asking the space about the ACL again. A
	// double that answers without touching the lock is what let this bug through review. The
	// watcher, status and notification components are mocked here, so their own paths are covered
	// by their own packages' tests - this tripwire only guards what the manager calls directly.
	readsAcl := func() bool {
		acl.RLock()
		defer acl.RUnlock()
		return acl.AclState().IsOneToOne()
	}
	fx.mockSpace.EXPECT().IsOneToOne().RunAndReturn(readsAcl).Maybe()
	fx.mockSpace.EXPECT().CanManageSpace().RunAndReturn(readsAcl).Maybe()
	fx.mockSpace.EXPECT().OnAclUpdated().Once()
	fx.mockStatus.EXPECT().GetLatestAclHeadId().Return("").Maybe()
	fx.mockStatus.EXPECT().SetOwner(mock.Anything, mock.Anything).Return(nil).Maybe()
	fx.mockStatus.EXPECT().SetAclInfo(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	fx.mockStatus.EXPECT().SetMyParticipantStatus(mock.Anything).Return(nil).Maybe()
	fx.mockStatus.EXPECT().GetLocalStatus().Return(spaceinfo.LocalStatusOk).Maybe()
	fx.mockParticipantWatcher.EXPECT().GetProcessedAclHeadId(mock.Anything, fx.mockSpace).Return("").Maybe()
	fx.mockParticipantWatcher.EXPECT().UpdateParticipantFromAclState(mock.Anything, fx.mockSpace, mock.Anything).Return(nil)
	// the forwarded flag has to be the one the locked state reports, not a hardcoded false: a
	// one-to-one member's state carries no RequestMetadata, so a wrong false would send it through
	// getSymKey and its identity would never register
	wantOneToOne := acl.AclState().IsOneToOne()
	var watchCalls atomic.Int32
	var wrongFlag atomic.Bool
	fx.mockParticipantWatcher.EXPECT().WatchParticipant(mock.Anything, fx.mockSpace, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ clientspace.Space, _ list.AccountState, isOneToOne bool) error {
			if isOneToOne != wantOneToOne {
				wrongFlag.Store(true)
			}
			watchCalls.Add(1)
			return nil
		})
	fx.mockAclNotification.EXPECT().AddRecords(acl, mock.Anything, "spaceId", mock.Anything, mock.Anything).Maybe()

	startupHead := acl.Head().Id
	fx.mockParticipantWatcher.EXPECT().SetProcessedAclHeadId(mock.Anything, fx.mockSpace, startupHead).Return(nil).Once()
	fx.run(t)
	<-fx.aclObjectManager.wait
	startupWatches := watchCalls.Load()

	// new records arrive from a peer: the head moves, so the callback has real work to do
	require.NoError(t, a.Execute("a.invite::invId2"))
	newHead := acl.Head().Id
	require.NotEqual(t, startupHead, newHead)
	fx.mockParticipantWatcher.EXPECT().SetProcessedAclHeadId(mock.Anything, fx.mockSpace, newHead).Return(nil).Once()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// exactly how any-sync enters the callback
		acl.Lock()
		defer acl.Unlock()
		fx.aclObjectManager.UpdateAcl(acl)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("UpdateAcl deadlocked: the callback path took the acl read lock while the write lock was held")
	}

	// not deadlocking is not enough: the callback has to have done the work. Without these, a
	// callback that returned early would pass this test.
	assert.False(t, wrongFlag.Load(), "WatchParticipant got an isOneToOne that disagrees with the acl state")
	assert.Greater(t, watchCalls.Load(), startupWatches, "the callback processed no participants")
	fx.aclObjectManager.mx.Lock()
	defer fx.aclObjectManager.mx.Unlock()
	assert.Equal(t, newHead, fx.aclObjectManager.lastIndexed)
}

// TestAclObjectManagerProcessesRemovalUnderAclWriteLock covers the removal fallback on the path
// any-sync drives: a sync response delivers our removal together with a newcomer's join request, and
// UpdateAcl runs with the ACL write lock held. The fallback must not take the ACL read lock either
// (GO-7525), and still has to mark the space deleted.
func TestAclObjectManagerProcessesRemovalUnderAclWriteLock(t *testing.T) {
	// given: b is an active reader at startup
	a := list.NewAclExecutor("spaceId")
	for _, cmd := range []string{"a.init::a", "a.invite::invId", "b.join::invId", "a.approve::b,r"} {
		require.NoError(t, a.Execute(cmd))
	}
	acl := &syncAclStub{AclList: a.ActualAccounts()["b"].Acl}

	fx := newFixture(t)
	defer fx.finish(t)
	fx.mockLoader.EXPECT().WaitLoad(mock.Anything).Return(fx.mockSpace, nil)
	fx.mockSpace.EXPECT().CommonSpace().Return(fx.mockCommonSpace)
	fx.mockSpace.EXPECT().Id().Return("spaceId")
	fx.mockCommonSpace.EXPECT().Acl().AnyTimes().Return(acl)
	fx.mockCommonSpace.EXPECT().Id().AnyTimes().Return("spaceId")
	fx.mockStatus.EXPECT().GetLatestAclHeadId().Return("").Times(2)
	fx.mockStatus.EXPECT().GetLocalStatus().Return(spaceinfo.LocalStatusOk)
	fx.mockStatus.EXPECT().SetOwner(a.ActualAccounts()["a"].Acl.AclState().Identity().Account(), mock.Anything).Return(nil).Once()
	fx.mockStatus.EXPECT().SetAclInfo(false, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	fx.mockStatus.EXPECT().SetMyParticipantStatus(model.ParticipantStatus_Active).Return(nil).Once()
	fx.mockParticipantWatcher.EXPECT().GetProcessedAclHeadId(mock.Anything, fx.mockSpace).Return("").Once()
	fx.mockParticipantWatcher.EXPECT().SetProcessedAclHeadId(mock.Anything, fx.mockSpace, acl.Head().Id).Return(nil).Once()
	// a and b at startup only: the removal must not process anyone's participant
	fx.mockParticipantWatcher.EXPECT().UpdateParticipantFromAclState(mock.Anything, fx.mockSpace, mock.Anything).Return(nil).Times(2)
	fx.mockParticipantWatcher.EXPECT().WatchParticipant(mock.Anything, fx.mockSpace, mock.Anything, mock.Anything).Return(nil).Times(2)
	fx.mockAclNotification.EXPECT().AddRecords(acl, list.AclPermissionsReader, "spaceId", spaceinfo.AccountStatusActive, spaceinfo.LocalStatusOk).Once()
	fx.run(t)
	<-fx.aclObjectManager.wait

	// the removal and c's join request, keyed to the read key the removal rotates in, arrive together
	for _, cmd := range []string{"a.remove::b", "a.invite::invId2", "c.join::invId2"} {
		require.NoError(t, a.Execute(cmd))
	}
	fx.mockSpace.EXPECT().OnAclUpdated().Once()

	// then
	fx.mockStatus.EXPECT().SetMyParticipantStatus(model.ParticipantStatus_Removed).Return(nil).Once()
	fx.mockStatus.EXPECT().SetPersistentStatus(spaceinfo.AccountStatusDeleted).Return(nil).Once()
	fx.mockAclNotification.EXPECT().AddRecords(acl, list.AclPermissionsNone, "spaceId", spaceinfo.AccountStatusDeleted, spaceinfo.LocalStatusOk).Once()

	// when
	done := make(chan struct{})
	go func() {
		defer close(done)
		// exactly how any-sync enters the callback
		acl.Lock()
		defer acl.Unlock()
		fx.aclObjectManager.UpdateAcl(acl)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("UpdateAcl deadlocked: the removal path took the acl read lock while the write lock was held")
	}
}

// TestAclObjectManagerForwardsOneToOne pins the other half of the GO-7525 handoff: the flag the
// callback passes down has to come from the locked ACL state. In a one-to-one space the derived
// owner must be skipped, and the members must be announced with isOneToOne true - their account
// states carry no RequestMetadata, so a hardcoded false would send them through getSymKey and
// their identities would never register.
func TestAclObjectManagerForwardsOneToOne(t *testing.T) {
	a := list.NewAclExecutor("spaceId")
	require.NoError(t, a.Execute("a;b.init-onetoone::a;b"))
	acl := &syncAclStub{AclList: a.ActualAccounts()["a"].Acl}
	require.True(t, acl.AclState().IsOneToOne(), "fixture must be a one-to-one acl")

	fx := newFixture(t)
	defer fx.finish(t)
	fx.mockLoader.EXPECT().WaitLoad(mock.Anything).Return(fx.mockSpace, nil)
	fx.mockSpace.EXPECT().CommonSpace().Return(fx.mockCommonSpace)
	fx.mockSpace.EXPECT().Id().Return("spaceId")
	fx.mockCommonSpace.EXPECT().Acl().AnyTimes().Return(acl)
	fx.mockCommonSpace.EXPECT().Id().AnyTimes().Return("spaceId")
	// the manager must not consult the space: it is under the acl lock
	fx.mockSpace.EXPECT().IsOneToOne().RunAndReturn(func() bool {
		acl.RLock()
		defer acl.RUnlock()
		return acl.AclState().IsOneToOne()
	}).Maybe()
	fx.mockStatus.EXPECT().GetLatestAclHeadId().Return("").Maybe()
	fx.mockStatus.EXPECT().SetOwner(mock.Anything, mock.Anything).Return(nil).Maybe()
	fx.mockStatus.EXPECT().SetAclInfo(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	fx.mockStatus.EXPECT().SetMyParticipantStatus(mock.Anything).Return(nil).Maybe()
	fx.mockStatus.EXPECT().GetLocalStatus().Return(spaceinfo.LocalStatusOk).Maybe()
	fx.mockParticipantWatcher.EXPECT().GetProcessedAclHeadId(mock.Anything, fx.mockSpace).Return("").Maybe()
	fx.mockParticipantWatcher.EXPECT().SetProcessedAclHeadId(mock.Anything, fx.mockSpace, mock.Anything).Return(nil).Maybe()

	var sawOwner atomic.Bool
	fx.mockParticipantWatcher.EXPECT().UpdateParticipantFromAclState(mock.Anything, fx.mockSpace, mock.Anything).
		RunAndReturn(func(_ context.Context, _ clientspace.Space, state list.AccountState) error {
			if state.Permissions.IsOwner() {
				sawOwner.Store(true)
			}
			return nil
		})
	var watched atomic.Int32
	var wrongFlag atomic.Bool
	fx.mockParticipantWatcher.EXPECT().WatchParticipant(mock.Anything, fx.mockSpace, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ clientspace.Space, _ list.AccountState, isOneToOne bool) error {
			if !isOneToOne {
				wrongFlag.Store(true)
			}
			watched.Add(1)
			return nil
		})
	fx.mockAclNotification.EXPECT().AddRecords(acl, mock.Anything, "spaceId", mock.Anything, mock.Anything).Maybe()
	fx.run(t)
	<-fx.aclObjectManager.wait

	assert.Positive(t, watched.Load(), "no participant was announced")
	assert.False(t, wrongFlag.Load(), "one-to-one members were announced with isOneToOne false")
	assert.False(t, sawOwner.Load(), "the derived one-to-one owner must not become a participant")
}
