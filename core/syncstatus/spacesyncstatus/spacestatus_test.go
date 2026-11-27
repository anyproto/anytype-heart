package spacesyncstatus

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/anyproto/any-sync/nodeconf/mock_nodeconf"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/account/mock_account"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/files/filespaceusage"
	"github.com/anyproto/anytype-heart/core/files/filesync"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/nodestatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/nodestatus/mock_nodestatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/spacesyncstatus/mock_spacesyncstatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/syncsubscriptions"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

var ctx = context.Background()

type mockSessionContext struct {
	id string
}

func (m mockSessionContext) ID() string {
	return m.id
}

func (m mockSessionContext) ObjectID() string {
	panic("implement me")
}

func (m mockSessionContext) TraceID() string {
	panic("implement me")
}

func (m mockSessionContext) SetMessages(smartBlockId string, msgs []*pb.EventMessage) {
	panic("implement me")
}

func (m mockSessionContext) GetMessages() []*pb.EventMessage {
	return nil
}

func (m mockSessionContext) GetResponseEvent() *pb.ResponseEvent {
	panic("implement me")
}

func (m mockSessionContext) IntegrationName() string {
	return ""
}

type fixture struct {
	*spaceSyncStatus
	a                   *app.App
	nodeConf            *mock_nodeconf.MockService
	nodeUsage           *mock_spacesyncstatus.MockNodeUsage
	nodeStatus          *mock_nodestatus.MockNodeStatus
	subscriptionService subscription.Service
	syncSubs            syncsubscriptions.SyncSubscriptions
	objectStore         *objectstore.StoreFixture
	spaceIdGetter       *mock_spacesyncstatus.MockSpaceIdGetter
	eventSender         *mock_event.MockSender
	session             session.HookRunner
	networkConfig       *mock_spacesyncstatus.MockNetworkConfig
	ctrl                *gomock.Controller
}

func genObject(syncStatus domain.ObjectSyncStatus, spaceId string) objectstore.TestObject {
	id := fmt.Sprintf("%d", rand.Int())
	return objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeySyncStatus:     domain.Int64(int64(syncStatus)),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		bundle.RelationKeyName:           domain.String("name" + id),
		bundle.RelationKeySpaceId:        domain.String(spaceId),
	}
}

func genSyncingObjects(fileObjects, objects int, spaceId string) []objectstore.TestObject {
	var res []objectstore.TestObject
	for i := 0; i < objects+fileObjects; i++ {
		res = append(res, genObject(domain.ObjectSyncStatusSyncing, spaceId))
	}
	return res
}

func newFixture(t *testing.T, beforeStart func(fx *fixture)) *fixture {
	a := new(app.App)
	ctrl := gomock.NewController(t)
	internalSubs := subscription.RegisterSubscriptionService(t, a)
	networkConfig := mock_spacesyncstatus.NewMockNetworkConfig(t)
	sess := session.NewHookRunner()
	fx := &fixture{
		a:                   a,
		ctrl:                ctrl,
		spaceSyncStatus:     NewSpaceSyncStatus().(*spaceSyncStatus),
		nodeUsage:           mock_spacesyncstatus.NewMockNodeUsage(t),
		nodeStatus:          mock_nodestatus.NewMockNodeStatus(t),
		nodeConf:            mock_nodeconf.NewMockService(ctrl),
		spaceIdGetter:       mock_spacesyncstatus.NewMockSpaceIdGetter(t),
		objectStore:         internalSubs.StoreFixture,
		eventSender:         app.MustComponent[event.Sender](a).(*mock_event.MockSender),
		subscriptionService: internalSubs,
		session:             sess,
		syncSubs:            syncsubscriptions.New(),
		networkConfig:       networkConfig,
	}
	accountService := mock_account.NewMockService(t)
	accountService.EXPECT().AccountID().Return("account1").Maybe()

	// Set startDelay to 0 for immediate execution in tests
	fx.spaceSyncStatus.startDelay = 0

	accountService.EXPECT().AccountID().Return("testAccountId").Maybe()

	a.Register(fx.syncSubs).
		Register(testutil.PrepareMock(ctx, a, networkConfig)).
		Register(testutil.PrepareMock(ctx, a, fx.nodeStatus)).
		Register(testutil.PrepareMock(ctx, a, fx.spaceIdGetter)).
		Register(testutil.PrepareMock(ctx, a, fx.nodeConf)).
		Register(testutil.PrepareMock(ctx, a, fx.nodeUsage)).
		Register(testutil.PrepareMock(ctx, a, accountService)).
		Register(sess).
		Register(fx.spaceSyncStatus)
	beforeStart(fx)
	err := a.Start(ctx)
	require.NoError(t, err)
	// Give the goroutine enough time to run
	time.Sleep(50 * time.Millisecond)
	return fx
}

func TestSpaceStatus(t *testing.T) {
	t.Run("empty space synced", func(t *testing.T) {
		fx := newFixture(t, func(fx *fixture) {
			fx.networkConfig.EXPECT().GetNetworkMode().Return(pb.RpcAccount_DefaultConfig).Maybe()
			fx.spaceIdGetter.EXPECT().AllLoadedSpaceIds().Return([]string{"spaceId"}).Maybe()
			fx.nodeStatus.EXPECT().GetNodeStatus("spaceId").Return(nodestatus.Online).Maybe()
			fx.nodeUsage.EXPECT().GetNodeUsage(mock.Anything).Return(&filespaceusage.NodeUsageResponse{
				Usage: filesync.NodeUsage{
					BytesLeft:         1000,
					AccountBytesLimit: 1000,
				},
				LocalUsageBytes: 0,
			}, nil).Maybe()
			fx.nodeConf.EXPECT().NetworkCompatibilityStatus().Return(nodeconf.NetworkCompatibilityStatusOk).AnyTimes() // gomock uses AnyTimes()
			fx.eventSender.EXPECT().Broadcast(event.NewEventSingleMessage("spaceId", &pb.EventMessageValueOfSpaceSyncStatusUpdate{
				SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
					Id:      "spaceId",
					Status:  pb.EventSpace_Synced,
					Network: pb.EventSpace_Anytype,
				},
			})).Once()
		})
		defer fx.ctrl.Finish()
	})
	t.Run("objects syncing", func(t *testing.T) {
		fx := newFixture(t, func(fx *fixture) {
			objs := genSyncingObjects(10, 100, "spaceId")
			fx.objectStore.AddObjects(t, "spaceId", objs)
			fx.networkConfig.EXPECT().GetNetworkMode().Return(pb.RpcAccount_DefaultConfig)
			fx.spaceIdGetter.EXPECT().AllLoadedSpaceIds().Return([]string{"spaceId"})
			fx.nodeStatus.EXPECT().GetNodeStatus("spaceId").Return(nodestatus.Online)
			fx.nodeUsage.EXPECT().GetNodeUsage(mock.Anything).Return(&filespaceusage.NodeUsageResponse{
				Usage: filesync.NodeUsage{
					BytesLeft:         1000,
					AccountBytesLimit: 1000,
				},
				LocalUsageBytes: 0,
			}, nil)
			fx.nodeConf.EXPECT().NetworkCompatibilityStatus().Return(nodeconf.NetworkCompatibilityStatusOk)
			fx.eventSender.EXPECT().Broadcast(event.NewEventSingleMessage("spaceId", &pb.EventMessageValueOfSpaceSyncStatusUpdate{
				SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
					Id:                    "spaceId",
					SyncingObjectsCounter: 110,
					Status:                pb.EventSpace_Syncing,
					Network:               pb.EventSpace_Anytype,
				},
			}))
		})
		defer fx.ctrl.Finish()
	})
	t.Run("objects syncing, not sending same event", func(t *testing.T) {
		fx := newFixture(t, func(fx *fixture) {
			fx.spaceSyncStatus.loopInterval = 10 * time.Millisecond
			objs := genSyncingObjects(10, 100, "spaceId")
			fx.objectStore.AddObjects(t, "spaceId", objs)
			fx.networkConfig.EXPECT().GetNetworkMode().Return(pb.RpcAccount_DefaultConfig)
			fx.spaceIdGetter.EXPECT().AllLoadedSpaceIds().Return([]string{"spaceId"})
			fx.nodeStatus.EXPECT().GetNodeStatus("spaceId").Return(nodestatus.Online)
			fx.nodeUsage.EXPECT().GetNodeUsage(mock.Anything).Return(&filespaceusage.NodeUsageResponse{
				Usage: filesync.NodeUsage{
					BytesLeft:         1000,
					AccountBytesLimit: 1000,
				},
				LocalUsageBytes: 0,
			}, nil)
			fx.nodeConf.EXPECT().NetworkCompatibilityStatus().AnyTimes().Return(nodeconf.NetworkCompatibilityStatusOk)
			fx.eventSender.EXPECT().Broadcast(event.NewEventSingleMessage("spaceId", &pb.EventMessageValueOfSpaceSyncStatusUpdate{
				SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
					Id:                    "spaceId",
					SyncingObjectsCounter: 110,
					Status:                pb.EventSpace_Syncing,
					Network:               pb.EventSpace_Anytype,
				},
			})).Times(1)
		})
		fx.Refresh("spaceId")
		time.Sleep(100 * time.Millisecond)
		defer fx.ctrl.Finish()
	})
	t.Run("local only", func(t *testing.T) {
		fx := newFixture(t, func(fx *fixture) {
			objs := genSyncingObjects(10, 100, "spaceId")
			fx.objectStore.AddObjects(t, "spaceId", objs)
			fx.networkConfig.EXPECT().GetNetworkMode().Return(pb.RpcAccount_LocalOnly)
			fx.spaceIdGetter.EXPECT().AllLoadedSpaceIds().Return([]string{"spaceId"})
			fx.eventSender.EXPECT().Broadcast(event.NewEventSingleMessage("spaceId", &pb.EventMessageValueOfSpaceSyncStatusUpdate{
				SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
					Id:      "spaceId",
					Status:  pb.EventSpace_Offline,
					Network: pb.EventSpace_LocalOnly,
				},
			}))
		})
		defer fx.ctrl.Finish()
	})
	t.Run("size exceeded", func(t *testing.T) {
		fx := newFixture(t, func(fx *fixture) {
			objs := genSyncingObjects(10, 100, "spaceId")
			fx.objectStore.AddObjects(t, "spaceId", objs)
			fx.networkConfig.EXPECT().GetNetworkMode().Return(pb.RpcAccount_DefaultConfig)
			fx.spaceIdGetter.EXPECT().AllLoadedSpaceIds().Return([]string{"spaceId"})
			fx.nodeStatus.EXPECT().GetNodeStatus("spaceId").Return(nodestatus.Online)
			fx.nodeUsage.EXPECT().GetNodeUsage(mock.Anything).Return(&filespaceusage.NodeUsageResponse{
				Usage: filesync.NodeUsage{
					BytesLeft:         1,
					AccountBytesLimit: 1000,
				},
				LocalUsageBytes: 0,
			}, nil)
			fx.nodeConf.EXPECT().NetworkCompatibilityStatus().Return(nodeconf.NetworkCompatibilityStatusOk)
			fx.eventSender.EXPECT().Broadcast(event.NewEventSingleMessage("spaceId", &pb.EventMessageValueOfSpaceSyncStatusUpdate{
				SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
					Id:                    "spaceId",
					SyncingObjectsCounter: 110,
					Status:                pb.EventSpace_Error,
					Network:               pb.EventSpace_Anytype,
					Error:                 pb.EventSpace_StorageLimitExceed,
				},
			}))
		})
		defer fx.ctrl.Finish()
	})
	t.Run("connection error", func(t *testing.T) {
		fx := newFixture(t, func(fx *fixture) {
			objs := genSyncingObjects(10, 100, "spaceId")
			fx.objectStore.AddObjects(t, "spaceId", objs)
			fx.networkConfig.EXPECT().GetNetworkMode().Return(pb.RpcAccount_DefaultConfig)
			fx.spaceIdGetter.EXPECT().AllLoadedSpaceIds().Return([]string{"spaceId"})
			fx.nodeStatus.EXPECT().GetNodeStatus("spaceId").Return(nodestatus.ConnectionError)
			fx.nodeUsage.EXPECT().GetNodeUsage(mock.Anything).Return(&filespaceusage.NodeUsageResponse{
				Usage: filesync.NodeUsage{
					BytesLeft:         1000,
					AccountBytesLimit: 1000,
				},
				LocalUsageBytes: 0,
			}, nil)
			fx.nodeConf.EXPECT().NetworkCompatibilityStatus().Return(nodeconf.NetworkCompatibilityStatusOk)
			fx.eventSender.EXPECT().Broadcast(event.NewEventSingleMessage("spaceId", &pb.EventMessageValueOfSpaceSyncStatusUpdate{
				SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
					Id:                    "spaceId",
					SyncingObjectsCounter: 110,
					Status:                pb.EventSpace_Offline,
					Network:               pb.EventSpace_Anytype,
					Error:                 pb.EventSpace_NetworkError,
				},
			}))
		})
		defer fx.ctrl.Finish()
	})
	t.Run("network incompatible", func(t *testing.T) {
		fx := newFixture(t, func(fx *fixture) {
			objs := genSyncingObjects(10, 100, "spaceId")
			fx.objectStore.AddObjects(t, "spaceId", objs)
			fx.networkConfig.EXPECT().GetNetworkMode().Return(pb.RpcAccount_DefaultConfig)
			fx.spaceIdGetter.EXPECT().AllLoadedSpaceIds().Return([]string{"spaceId"})
			fx.nodeStatus.EXPECT().GetNodeStatus("spaceId").Return(nodestatus.ConnectionError)
			fx.nodeUsage.EXPECT().GetNodeUsage(mock.Anything).Return(&filespaceusage.NodeUsageResponse{
				Usage: filesync.NodeUsage{
					BytesLeft:         1000,
					AccountBytesLimit: 1000,
				},
				LocalUsageBytes: 0,
			}, nil)
			fx.nodeConf.EXPECT().NetworkCompatibilityStatus().Return(nodeconf.NetworkCompatibilityStatusIncompatible)
			fx.eventSender.EXPECT().Broadcast(event.NewEventSingleMessage("spaceId", &pb.EventMessageValueOfSpaceSyncStatusUpdate{
				SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
					Id:                    "spaceId",
					SyncingObjectsCounter: 110,
					Status:                pb.EventSpace_Error,
					Network:               pb.EventSpace_Anytype,
					Error:                 pb.EventSpace_IncompatibleVersion,
				},
			}))
		})
		defer fx.ctrl.Finish()
	})
	t.Run("objects syncing, refresh with missing objects", func(t *testing.T) {
		fx := newFixture(t, func(fx *fixture) {
			fx.spaceSyncStatus.loopInterval = 10 * time.Millisecond
			objs := genSyncingObjects(10, 100, "spaceId")
			fx.objectStore.AddObjects(t, "spaceId", objs)
			fx.networkConfig.EXPECT().GetNetworkMode().Return(pb.RpcAccount_DefaultConfig)
			fx.spaceIdGetter.EXPECT().AllLoadedSpaceIds().Return([]string{"spaceId"})
			fx.nodeStatus.EXPECT().GetNodeStatus("spaceId").Return(nodestatus.Online)
			fx.nodeUsage.EXPECT().GetNodeUsage(mock.Anything).Return(&filespaceusage.NodeUsageResponse{
				Usage: filesync.NodeUsage{
					BytesLeft:         1000,
					AccountBytesLimit: 1000,
				},
				LocalUsageBytes: 0,
			}, nil)
			fx.nodeConf.EXPECT().NetworkCompatibilityStatus().AnyTimes().Return(nodeconf.NetworkCompatibilityStatusOk)
			fx.eventSender.EXPECT().Broadcast(event.NewEventSingleMessage("spaceId", &pb.EventMessageValueOfSpaceSyncStatusUpdate{
				SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
					Id:                    "spaceId",
					SyncingObjectsCounter: 110,
					Status:                pb.EventSpace_Syncing,
					Network:               pb.EventSpace_Anytype,
				},
			}))
		})
		fx.UpdateMissingIds("spaceId", []string{"missingId"})
		fx.Refresh("spaceId")
		fx.eventSender.EXPECT().Broadcast(event.NewEventSingleMessage("spaceId", &pb.EventMessageValueOfSpaceSyncStatusUpdate{
			SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
				Id:                    "spaceId",
				SyncingObjectsCounter: 111,
				Status:                pb.EventSpace_Syncing,
				Network:               pb.EventSpace_Anytype,
			},
		})).Times(1)
		time.Sleep(100 * time.Millisecond)
		defer fx.ctrl.Finish()
	})
	t.Run("sync protocol compatibility", func(t *testing.T) {
		fx := newFixture(t, func(fx *fixture) {
			fx.networkConfig.EXPECT().GetNetworkMode().Return(pb.RpcAccount_DefaultConfig)
			fx.spaceIdGetter.EXPECT().AllLoadedSpaceIds().Return([]string{"spaceId"})
			fx.nodeStatus.EXPECT().GetNodeStatus("spaceId").Return(nodestatus.Online)
			fx.nodeUsage.EXPECT().GetNodeUsage(mock.Anything).Return(&filespaceusage.NodeUsageResponse{
				Usage: filesync.NodeUsage{
					BytesLeft:         1000,
					AccountBytesLimit: 1000,
				},
				LocalUsageBytes: 0,
			}, nil)
			fx.nodeConf.EXPECT().NetworkCompatibilityStatus().Return(nodeconf.NetworkCompatibilityStatusNeedsUpdate)
			fx.eventSender.EXPECT().Broadcast(event.NewEventSingleMessage("spaceId", &pb.EventMessageValueOfSpaceSyncStatusUpdate{
				SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
					Id:      "spaceId",
					Status:  pb.EventSpace_NetworkNeedsUpdate,
					Network: pb.EventSpace_Anytype,
				},
			}))
		})
		defer fx.ctrl.Finish()
	})
	t.Run("hook new session", func(t *testing.T) {
		fx := newFixture(t, func(fx *fixture) {
			objs := genSyncingObjects(10, 100, "spaceId")
			fx.objectStore.AddObjects(t, "spaceId", objs)
			fx.networkConfig.EXPECT().GetNetworkMode().Return(pb.RpcAccount_DefaultConfig)
			fx.spaceIdGetter.EXPECT().AllLoadedSpaceIds().Return([]string{"spaceId"})
			fx.nodeStatus.EXPECT().GetNodeStatus("spaceId").Return(nodestatus.ConnectionError)
			fx.nodeUsage.EXPECT().GetNodeUsage(mock.Anything).Return(&filespaceusage.NodeUsageResponse{
				Usage: filesync.NodeUsage{
					BytesLeft:         1000,
					AccountBytesLimit: 1000,
				},
				LocalUsageBytes: 0,
			}, nil)
			fx.nodeConf.EXPECT().NetworkCompatibilityStatus().AnyTimes().Return(nodeconf.NetworkCompatibilityStatusIncompatible)
			fx.eventSender.EXPECT().Broadcast(event.NewEventSingleMessage("spaceId", &pb.EventMessageValueOfSpaceSyncStatusUpdate{
				SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
					Id:                    "spaceId",
					SyncingObjectsCounter: 110,
					Status:                pb.EventSpace_Error,
					Network:               pb.EventSpace_Anytype,
					Error:                 pb.EventSpace_IncompatibleVersion,
				},
			}))
		})
		fx.eventSender.EXPECT().SendToSession("sessionId", event.NewEventSingleMessage("spaceId", &pb.EventMessageValueOfSpaceSyncStatusUpdate{
			SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
				Id:                    "spaceId",
				SyncingObjectsCounter: 110,
				Status:                pb.EventSpace_Error,
				Network:               pb.EventSpace_Anytype,
				Error:                 pb.EventSpace_IncompatibleVersion,
			},
		}))
		fx.session.RunHooks(mockSessionContext{"sessionId"})
		defer fx.ctrl.Finish()
	})
	t.Run("hook new session local only", func(t *testing.T) {
		fx := newFixture(t, func(fx *fixture) {
			objs := genSyncingObjects(10, 100, "spaceId")
			fx.objectStore.AddObjects(t, "spaceId", objs)
			fx.networkConfig.EXPECT().GetNetworkMode().Return(pb.RpcAccount_LocalOnly)
			fx.spaceIdGetter.EXPECT().AllLoadedSpaceIds().Return([]string{"spaceId"})
			fx.eventSender.EXPECT().Broadcast(event.NewEventSingleMessage("spaceId", &pb.EventMessageValueOfSpaceSyncStatusUpdate{
				SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
					Id:      "spaceId",
					Status:  pb.EventSpace_Offline,
					Network: pb.EventSpace_LocalOnly,
				},
			}))
		})
		fx.eventSender.EXPECT().SendToSession("sessionId", event.NewEventSingleMessage("spaceId", &pb.EventMessageValueOfSpaceSyncStatusUpdate{
			SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
				Id:      "spaceId",
				Status:  pb.EventSpace_Offline,
				Network: pb.EventSpace_LocalOnly,
			},
		}))
		fx.session.RunHooks(mockSessionContext{"sessionId"})
		defer fx.ctrl.Finish()
	})
}

// noSubscriptions fails the test when a subscription is asked for.
type noSubscriptions struct {
	syncsubscriptions.SyncSubscriptions
	t *testing.T
}

func (n noSubscriptions) GetSubscription(spaceId string) (syncsubscriptions.SyncSubscription, error) {
	n.t.Errorf("a status read asked for the subscriptions of %s", spaceId)
	return nil, fmt.Errorf("no subscriptions in this test")
}

func TestSpaceSyncStatus_SpaceStatus(t *testing.T) {
	fileObject := func(id string, status filesyncstatus.Status, creator string) objectstore.TestObject {
		return objectstore.TestObject{
			bundle.RelationKeyId:               domain.String(id),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_file)),
			bundle.RelationKeyFileBackupStatus: domain.Int64(int64(status)),
			bundle.RelationKeyCreator:          domain.String(creator),
		}
	}
	// newSpaceFixture seeds three objects not in sync, one of this account's
	// files uploading and another member's file refused for the limit, and
	// swaps the subscriptions for a stand-in that fails on use
	newSpaceFixture := func(t *testing.T, checked bool) *fixture {
		fx := newFixture(t, func(fx *fixture) {
			fx.objectStore.AddObjects(t, "spaceId", genSyncingObjects(0, 3, "spaceId"))
			fx.objectStore.AddObjects(t, "spaceId", []objectstore.TestObject{
				fileObject("file-uploading", filesyncstatus.Queued, domain.NewParticipantId("spaceId", "account1")),
				fileObject("file-uploading-2", filesyncstatus.Syncing, domain.NewParticipantId("spaceId", "account1")),
				fileObject("file-refused", filesyncstatus.Limited, domain.NewParticipantId("spaceId", "someoneElse")),
			})
			fx.networkConfig.EXPECT().GetNetworkMode().Return(pb.RpcAccount_DefaultConfig).Maybe()
			fx.spaceIdGetter.EXPECT().AllLoadedSpaceIds().Return(nil).Maybe()
			fx.nodeStatus.EXPECT().TryGetNodeStatus("spaceId").Return(nodestatus.Online, checked)
			fx.nodeStatus.EXPECT().GetNodeStatus("spaceId").Return(nodestatus.Online)
			fx.nodeConf.EXPECT().NetworkCompatibilityStatus().Return(nodeconf.NetworkCompatibilityStatusOk)
		})
		fx.spaceSyncStatus.subs = noSubscriptions{t: t}
		return fx
	}

	t.Run("counts from the store, without subscriptions, broadcasts or the account usage", func(t *testing.T) {
		// given: the event sender mock fails the test on any Broadcast, the
		// node usage mock on any GetNodeUsage (the account usage can wait on
		// the backup node), and the stand-in on any subscription
		fx := newSpaceFixture(t, true)
		defer fx.ctrl.Finish()
		// two objects the node holds that this device has not downloaded
		fx.UpdateMissingIds("spaceId", []string{"missing1", "missing2"})
		want := &pb.EventSpaceSyncStatusUpdate{
			Id:                    "spaceId",
			Status:                pb.EventSpace_Syncing,
			Network:               pb.EventSpace_Anytype,
			SyncingObjectsCounter: 5,
			UploadingFilesCounter: 2,
			NotSyncedFilesCounter: 1,
		}

		// when
		got, checked, err := fx.SpaceStatus("spaceId")

		// then
		require.NoError(t, err)
		require.Equal(t, want, got)
		require.True(t, checked)
		// the broadcast dedup is untouched: a read must not suppress the
		// next status the app is sent
		fx.mx.Lock()
		require.Empty(t, fx.lastSentEvents)
		fx.mx.Unlock()
	})
	t.Run("a connection not tried yet is reported unchecked", func(t *testing.T) {
		// given
		fx := newSpaceFixture(t, false)
		defer fx.ctrl.Finish()

		// when
		_, checked, err := fx.SpaceStatus("spaceId")

		// then
		require.NoError(t, err)
		require.False(t, checked)
	})
	t.Run("local only", func(t *testing.T) {
		// given: in local-only mode nothing but the network mode is read
		fx := newFixture(t, func(fx *fixture) {
			fx.networkConfig.EXPECT().GetNetworkMode().Return(pb.RpcAccount_LocalOnly)
			fx.spaceIdGetter.EXPECT().AllLoadedSpaceIds().Return(nil).Maybe()
		})
		defer fx.ctrl.Finish()
		want := &pb.EventSpaceSyncStatusUpdate{
			Id:      "spaceId",
			Status:  pb.EventSpace_Offline,
			Network: pb.EventSpace_LocalOnly,
		}

		// when
		got, checked, err := fx.SpaceStatus("spaceId")

		// then
		require.NoError(t, err)
		require.Equal(t, want, got)
		require.True(t, checked)
	})
}
