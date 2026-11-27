package v2service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// syncFixture is the v2 fixture with the sync port set. The mock is strict:
// a call the test did not expect fails it, which is how a local-only read
// that asks for storage, or a 404 that reads the status, would show.
func newSyncFixture(t *testing.T) (*v2Fixture, *mock_apicore.MockSyncStatus) {
	fx := newV2Fixture(t)
	syncStatus := mock_apicore.NewMockSyncStatus(t)
	fx.syncStatus = syncStatus
	fx.addType(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:           domain.String("type-page"),
		bundle.RelationKeyUniqueKey:    domain.String("ot-page"),
		bundle.RelationKeyApiObjectKey: domain.String("page"),
		bundle.RelationKeyName:         domain.String("Page"),
	})
	return fx, syncStatus
}

func (fx *v2Fixture) addPendingObject(t *testing.T, id string, status domain.ObjectSyncStatus, syncErr domain.SyncError, modified int64) {
	obj := objectstore.TestObject{
		bundle.RelationKeyId:               domain.String(id),
		bundle.RelationKeyName:             domain.String("Name " + id),
		bundle.RelationKeyType:             domain.String("type-page"),
		bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
		bundle.RelationKeySyncStatus:       domain.Int64(int64(status)),
		bundle.RelationKeyLastModifiedDate: domain.Int64(modified),
	}
	// most objects carry no syncError at all: the filter excluding oversized
	// objects must keep them
	if syncErr != domain.SyncErrorNull {
		obj[bundle.RelationKeySyncError] = domain.Int64(int64(syncErr))
	}
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{obj})
}

// fileOrder gives each added file a later creation date, so the waiting
// files are taken in the order a test adds them.
var fileOrder atomic.Int64

// addFile adds a file object in the given backup state, created by creator.
func (fx *v2Fixture) addFile(t *testing.T, id string, status filesyncstatus.Status, size int64, creator string) {
	syncStatus, syncErr := domain.ObjectSyncStatusSynced, domain.SyncErrorNull
	if status == filesyncstatus.Limited {
		// what the details updater sets on a file refused for the limit
		syncStatus, syncErr = domain.ObjectSyncStatusError, domain.SyncErrorOversized
	}
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:               domain.String(id),
		bundle.RelationKeyName:             domain.String(id),
		bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_file)),
		bundle.RelationKeyFileBackupStatus: domain.Int64(int64(status)),
		bundle.RelationKeySizeInBytes:      domain.Int64(size),
		bundle.RelationKeyCreator:          domain.String(creator),
		bundle.RelationKeyCreatedDate:      domain.Int64(fileOrder.Add(1)),
		bundle.RelationKeySyncStatus:       domain.Int64(int64(syncStatus)),
		bundle.RelationKeySyncError:        domain.Int64(int64(syncErr)),
	}})
}

func (fx *v2Fixture) setOwnPermissions(t *testing.T, permissions model.ParticipantPermissions) {
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:                     domain.String(domain.NewParticipantId(testSpaceId, testAccountId)),
		bundle.RelationKeyResolvedLayout:         domain.Int64(int64(model.ObjectType_participant)),
		bundle.RelationKeyParticipantPermissions: domain.Int64(int64(permissions)),
	}})
}

// makeOneToOne marks the test space's view as a one-to-one space, by the
// earlier marker.
func (fx *v2Fixture) makeOneToOne(t *testing.T) {
	fx.markSpaceView(t, bundle.RelationKeySpaceUxType, int64(model.SpaceUxType_OneToOne))
}

func (fx *v2Fixture) markSpaceView(t *testing.T, key domain.RelationKey, value int64) {
	fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String("spaceView_" + testSpaceId),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
		bundle.RelationKeyTargetSpaceId:  domain.String(testSpaceId),
		key:                              domain.Int64(value),
	}})
}

func myParticipant() string {
	return domain.NewParticipantId(testSpaceId, testAccountId)
}

func otherParticipant() string {
	return domain.NewParticipantId(testSpaceId, "someoneElse")
}

// reported is a fresh quota report of an account whose whole limit its
// spaces share.
func reported(limit, used int64) apicore.SpaceStorage {
	return apicore.SpaceStorage{LimitBytes: limit, NodeUsedBytes: used, ReportedAt: time.Now(), PlanLimit: true}
}

func TestGetSyncStatus(t *testing.T) {
	get := func(t *testing.T, fx *v2Fixture, withObjects bool) *v2model.SyncStatus {
		got, err := fx.GetSyncStatus(context.Background(), testSpaceId, withObjects, 0, 25)
		require.NoError(t, err)
		return got
	}
	// expect answers the two sync reads; a test passing no quota expects
	// none to be reported
	expect := func(syncStatus *mock_apicore.MockSyncStatus, event *pb.EventSpaceSyncStatusUpdate, quota *apicore.SpaceStorage) {
		syncStatus.EXPECT().SpaceSyncStatus(testSpaceId).Return(event, true, nil)
		if quota == nil {
			syncStatus.EXPECT().SpaceStorage(testSpaceId).Return(apicore.SpaceStorage{}, false, nil)
			return
		}
		syncStatus.EXPECT().SpaceStorage(testSpaceId).Return(*quota, true, nil)
	}
	quota := func(q apicore.SpaceStorage) *apicore.SpaceStorage { return &q }
	anytype := &pb.EventSpaceSyncStatusUpdate{Network: pb.EventSpace_Anytype}

	t.Run("lists what is out of sync, the most recently changed first", func(t *testing.T) {
		// given: three pending objects, one synced, an object too big to sync
		// (never syncs, the app does not count it), one in the Bin, and a file
		// refused for the limit, which the app counts apart
		fx, syncStatus := newSyncFixture(t)
		fx.addPendingObject(t, "obj-old", domain.ObjectSyncStatusSyncing, domain.SyncErrorNull, 100)
		fx.addPendingObject(t, "obj-new", domain.ObjectSyncStatusQueued, domain.SyncErrorNull, 300)
		fx.addPendingObject(t, "obj-err", domain.ObjectSyncStatusError, domain.SyncErrorNetworkError, 200)
		fx.addPendingObject(t, "obj-synced", domain.ObjectSyncStatusSynced, domain.SyncErrorNull, 400)
		fx.addPendingObject(t, "obj-oversized", domain.ObjectSyncStatusError, domain.SyncErrorOversized, 500)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:               domain.String("obj-archived"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeySyncStatus:       domain.Int64(int64(domain.ObjectSyncStatusSyncing)),
			bundle.RelationKeyLastModifiedDate: domain.Int64(600),
			bundle.RelationKeyIsArchived:       domain.Bool(true),
		}})
		fx.addFile(t, "file-refused", filesyncstatus.Limited, 30, otherParticipant())
		fx.addFile(t, "file-queued", filesyncstatus.Queued, 50, myParticipant())
		expect(syncStatus, &pb.EventSpaceSyncStatusUpdate{
			Id:                    testSpaceId,
			Status:                pb.EventSpace_Syncing,
			Network:               pb.EventSpace_Anytype,
			SyncingObjectsCounter: 5, // the 3 above and 2 not downloaded yet
			UploadingFilesCounter: 1,
		}, quota(reported(1000, 100)))
		want := &v2model.SyncStatus{
			SpaceId:              testSpaceId,
			Status:               v2model.SyncStatusSyncing,
			Network:              v2model.SyncNetworkAnytype,
			PendingObjectCount:   5,
			UploadingFileCount:   1,
			NotBackedUpFileCount: 1,
			Storage:              &v2model.Storage{UsedBytes: 100, PendingBytes: 50, LimitBytes: 1000, LeftBytes: 850},
			Objects: []v2model.PendingObject{
				{Id: "obj-new", Name: "Name obj-new", Type: "page", Status: v2model.PendingObjectQueued},
				{Id: "obj-err", Name: "Name obj-err", Type: "page", Status: v2model.PendingObjectError, Error: v2model.SyncErrorNetwork},
				{Id: "obj-old", Name: "Name obj-old", Type: "page", Status: v2model.PendingObjectSyncing},
				{Id: "file-refused", Name: "file-refused", Status: v2model.PendingObjectNotBackedUp},
			},
			Notes: []string{"1 file did not fit in the storage limit and is not backed up. There is room now: they upload from the device that added them."},
		}

		// when
		got := get(t, fx, true)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("pages the list", func(t *testing.T) {
		// given
		fx, syncStatus := newSyncFixture(t)
		fx.addPendingObject(t, "obj1", domain.ObjectSyncStatusSyncing, domain.SyncErrorNull, 300)
		fx.addPendingObject(t, "obj2", domain.ObjectSyncStatusSyncing, domain.SyncErrorNull, 200)
		fx.addPendingObject(t, "obj3", domain.ObjectSyncStatusSyncing, domain.SyncErrorNull, 100)
		expect(syncStatus, &pb.EventSpaceSyncStatusUpdate{Status: pb.EventSpace_Syncing, SyncingObjectsCounter: 3}, nil)

		// when
		got, err := fx.GetSyncStatus(context.Background(), testSpaceId, true, 1, 1)

		// then
		require.NoError(t, err)
		require.Len(t, got.Objects, 1)
		assert.Equal(t, "obj2", got.Objects[0].Id)
		assert.True(t, got.HasMore)
	})

	t.Run("counts alone by default", func(t *testing.T) {
		// given
		fx, syncStatus := newSyncFixture(t)
		fx.addPendingObject(t, "obj1", domain.ObjectSyncStatusSyncing, domain.SyncErrorNull, 100)
		expect(syncStatus, &pb.EventSpaceSyncStatusUpdate{Status: pb.EventSpace_Syncing, SyncingObjectsCounter: 1}, nil)

		// when
		got := get(t, fx, false)

		// then
		assert.Equal(t, 1, got.PendingObjectCount)
		assert.Nil(t, got.Objects)
		assert.False(t, got.HasMore)
	})

	t.Run("uploads confirmed since the report count once", func(t *testing.T) {
		// given: two uploads confirmed since the node's report; one still
		// reads waiting in the store, so it is counted there and not again
		fx, syncStatus := newSyncFixture(t)
		fx.addFile(t, "file-still-waiting", filesyncstatus.Syncing, 200, myParticipant())
		q := reported(1000, 100)
		q.Confirmed = map[string]apicore.ConfirmedUpload{"file-done": {Bytes: 300}, "file-still-waiting": {Bytes: 200}}
		expect(syncStatus, anytype, &q)

		// when
		got := get(t, fx, false)

		// then
		assert.Equal(t, &v2model.Storage{UsedBytes: 400, PendingBytes: 200, LimitBytes: 1000, LeftBytes: 400}, got.Storage)
	})

	t.Run("an upload completing between the two reads is still counted", func(t *testing.T) {
		// given: the snapshot is taken before the confirmation; by then the
		// store already reads the file synced. Read in that order, the file
		// would be in neither.
		fx, syncStatus := newSyncFixture(t)
		fx.addFile(t, "file-completing", filesyncstatus.Syncing, 300, myParticipant())
		syncStatus.EXPECT().SpaceSyncStatus(testSpaceId).Return(anytype, true, nil)
		syncStatus.EXPECT().SpaceStorage(testSpaceId).RunAndReturn(func(string) (apicore.SpaceStorage, bool, error) {
			fx.addFile(t, "file-completing", filesyncstatus.Synced, 300, myParticipant())
			return reported(1000, 100), true, nil
		})

		// when
		got := get(t, fx, false)

		// then
		assert.Equal(t, &v2model.Storage{UsedBytes: 100, PendingBytes: 300, LimitBytes: 1000, LeftBytes: 600}, got.Storage)
	})

	t.Run("an upload a report already includes is not counted again while it still reads waiting", func(t *testing.T) {
		// given: the node's usage includes the file; the store has not caught up
		fx, syncStatus := newSyncFixture(t)
		fx.addFile(t, "file-reported", filesyncstatus.Syncing, 200, myParticipant())
		q := reported(1000, 400)
		q.Confirmed = map[string]apicore.ConfirmedUpload{"file-reported": {Bytes: 200, InReport: true}}
		expect(syncStatus, anytype, &q)

		// when
		got := get(t, fx, false)

		// then
		assert.Equal(t, &v2model.Storage{UsedBytes: 400, PendingBytes: 0, LimitBytes: 1000, LeftBytes: 600}, got.Storage)
	})

	t.Run("a waiting file the report may hold is not taken off the room again", func(t *testing.T) {
		// given: the node reports 700, which already holds a 200-byte upload
		// whose confirmation came after the report and whose row still reads
		// syncing; a refused 150-byte file fits the 300 the node shows free
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
		fx.addFile(t, "file-done", filesyncstatus.Syncing, 200, myParticipant())
		fx.addFile(t, "file-refused", filesyncstatus.Limited, 150, myParticipant())
		q := reported(1000, 700)
		q.Confirmed = map[string]apicore.ConfirmedUpload{"file-done": {Bytes: 200}}
		expect(syncStatus, anytype, &q)

		// when
		got := get(t, fx, false)

		// then
		assert.False(t, got.UpgradeNeeded)
	})

	t.Run("local bookkeeping alone never recommends buying", func(t *testing.T) {
		// given: a confirmation the node's report may already hold pushes the
		// estimate past the limit; the node's own report does not
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
		q := reported(1000, 500)
		q.Confirmed = map[string]apicore.ConfirmedUpload{"file-done": {Bytes: 600}}
		expect(syncStatus, anytype, &q)

		// when
		got := get(t, fx, false)

		// then
		assert.Equal(t, int64(1100), got.Storage.UsedBytes)
		assert.False(t, got.UpgradeNeeded)
		assert.Empty(t, got.Notes)
	})

	t.Run("the owner with refused files that do not fit: upgrade needed", func(t *testing.T) {
		// given: a refused file, and two waiting ones of which only the second
		// fits; another member's waiting file is theirs to count. The app's
		// storage warning does not turn the status into an error.
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
		fx.addFile(t, "file-queued", filesyncstatus.Queued, 300, myParticipant())
		fx.addFile(t, "file-syncing", filesyncstatus.Syncing, 100, myParticipant())
		fx.addFile(t, "file-synced", filesyncstatus.Synced, 5000, myParticipant())
		fx.addFile(t, "file-refused", filesyncstatus.Limited, 7000, myParticipant())
		fx.addFile(t, "file-other-member", filesyncstatus.Queued, 9000, otherParticipant())
		expect(syncStatus, &pb.EventSpaceSyncStatusUpdate{
			Status:  pb.EventSpace_Error,
			Error:   pb.EventSpace_StorageLimitExceed,
			Network: pb.EventSpace_Anytype,
		}, quota(reported(1000, 900)))
		want := &v2model.SyncStatus{
			SpaceId:              testSpaceId,
			Status:               v2model.SyncStatusSynced,
			Network:              v2model.SyncNetworkAnytype,
			NotBackedUpFileCount: 1,
			UpgradeNeeded:        true,
			Storage:              &v2model.Storage{UsedBytes: 900, PendingBytes: 400, LimitBytes: 1000, LeftBytes: 0},
			Notes:                []string{"1 file did not fit in the storage limit and is not backed up. Upgrade the plan or delete files to free space."},
		}

		// when
		got := get(t, fx, false)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("a refused file with room again needs no upgrade", func(t *testing.T) {
		// given: the plan was upgraded since the files were refused; the
		// devices that hold them upload them on their own
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
		fx.addFile(t, "file-refused-a", filesyncstatus.Limited, 50, otherParticipant())
		fx.addFile(t, "file-refused-b", filesyncstatus.Limited, 50, myParticipant())
		expect(syncStatus, anytype, quota(reported(100_000, 900)))

		// when
		got := get(t, fx, false)

		// then
		assert.Equal(t, 2, got.NotBackedUpFileCount)
		assert.False(t, got.UpgradeNeeded)
		assert.Equal(t, []string{"2 files did not fit in the storage limit and are not backed up. There is room now: they upload from the device that added them."}, got.Notes)
	})

	t.Run("refused files before the quota is reported: no remedy is guessed", func(t *testing.T) {
		// given: the refusal may predate an upgrade
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
		fx.addFile(t, "file-refused", filesyncstatus.Limited, 300, myParticipant())
		expect(syncStatus, anytype, nil)

		// when
		got := get(t, fx, false)

		// then
		assert.False(t, got.UpgradeNeeded)
		assert.Equal(t, []string{"1 file did not fit in the storage limit and is not backed up."}, got.Notes)
	})

	t.Run("an owner at the limit with nothing waiting", func(t *testing.T) {
		for name, q := range map[string]apicore.SpaceStorage{
			"over the limit": reported(1000, 1500),
			"a zero limit":   reported(0, 0),
		} {
			t.Run(name, func(t *testing.T) {
				// given: any new file is refused
				fx, syncStatus := newSyncFixture(t)
				fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
				expect(syncStatus, anytype, &q)

				// when
				got := get(t, fx, false)

				// then
				assert.True(t, got.UpgradeNeeded)
				assert.Equal(t, []string{"The storage limit is reached: new files will not be backed up. Upgrade the plan or delete files to free space."}, got.Notes)
			})
		}
	})

	t.Run("a stale report recommends no upgrade", func(t *testing.T) {
		// given: the device has not reached the node for an hour; the plan
		// may have changed since
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
		q := reported(1000, 1500)
		q.ReportedAt = time.Now().Add(-time.Hour)
		expect(syncStatus, anytype, &q)

		// when
		got := get(t, fx, false)

		// then
		assert.False(t, got.UpgradeNeeded)
		assert.Equal(t, []string{"The storage limit was reached at the backup node's last report."}, got.Notes)
	})

	t.Run("an account whose limit is split, or not known to be whole: no purchase advised", func(t *testing.T) {
		// given: the account report does not show the whole limit shared by
		// its spaces, so a plan upgrade may not raise this space's
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
		q := reported(1000, 1000)
		q.PlanLimit = false
		expect(syncStatus, anytype, &q)

		// when
		got := get(t, fx, false)

		// then
		assert.False(t, got.UpgradeNeeded)
		assert.Equal(t, []string{"The storage limit is reached: new files will not be backed up. Delete files to free space."}, got.Notes)
	})

	t.Run("a member of someone else's space is not told to upgrade", func(t *testing.T) {
		// given
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Writer)
		fx.addFile(t, "file-refused", filesyncstatus.Limited, 300, myParticipant())
		expect(syncStatus, anytype, quota(reported(1000, 1000)))

		// when
		got := get(t, fx, false)

		// then
		assert.False(t, got.UpgradeNeeded)
		assert.Equal(t, []string{"1 file did not fit in the storage limit and is not backed up. The limit is the space owner's: delete files you added to free space, or ask the owner for more room."}, got.Notes)
	})

	t.Run("a membership not indexed yet names no payer", func(t *testing.T) {
		// given: no participant row for the caller yet
		fx, syncStatus := newSyncFixture(t)
		expect(syncStatus, anytype, quota(reported(1000, 1000)))

		// when
		got := get(t, fx, false)

		// then
		assert.False(t, got.UpgradeNeeded)
		assert.Equal(t, []string{"The storage limit is reached: new files will not be backed up. Delete files to free space, or ask the space owner for more room."}, got.Notes)
	})

	t.Run("in a one-to-one space the caller pays for their own files", func(t *testing.T) {
		// given: the caller is no owner (a one-to-one space has none among
		// its participants); both members' refused files are listed, but only
		// the caller's count against the caller's storage
		fx, syncStatus := newSyncFixture(t)
		fx.makeOneToOne(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Writer)
		fx.addFile(t, "file-refused-mine", filesyncstatus.Limited, 300, myParticipant())
		fx.addFile(t, "file-refused-theirs", filesyncstatus.Limited, 5000, otherParticipant())
		expect(syncStatus, anytype, quota(reported(1000, 900)))

		// when
		got := get(t, fx, true)

		// then
		assert.Equal(t, 2, got.NotBackedUpFileCount)
		assert.Len(t, got.Objects, 2)
		assert.True(t, got.UpgradeNeeded)
		assert.Equal(t, []string{"2 files did not fit in the storage limit and are not backed up. In a one-to-one space each member's files count against their own storage. Upgrade the plan or delete files to free space."}, got.Notes)
	})

	t.Run("the current one-to-one marker alone makes the caller the payer", func(t *testing.T) {
		// given: only spaceType marks the space
		fx, syncStatus := newSyncFixture(t)
		fx.markSpaceView(t, bundle.RelationKeySpaceType, int64(model.SpaceType_SpaceTypeOneToOne))
		fx.addFile(t, "file-refused-mine", filesyncstatus.Limited, 300, myParticipant())
		expect(syncStatus, anytype, quota(reported(1000, 900)))

		// when
		got := get(t, fx, false)

		// then
		assert.True(t, got.UpgradeNeeded)
	})

	t.Run("a one-to-one space with room for the caller's refused file", func(t *testing.T) {
		// given: both members have a refused file; the caller has room again
		fx, syncStatus := newSyncFixture(t)
		fx.makeOneToOne(t)
		fx.addFile(t, "file-refused-mine", filesyncstatus.Limited, 100, myParticipant())
		fx.addFile(t, "file-refused-theirs", filesyncstatus.Limited, 100, otherParticipant())
		expect(syncStatus, anytype, quota(reported(1000, 100)))

		// when
		got := get(t, fx, false)

		// then
		assert.Equal(t, []string{"2 files did not fit in the storage limit and are not backed up. In a one-to-one space each member's files count against their own storage. There is room now for yours: they upload from the device that added them."}, got.Notes)
	})

	t.Run("a one-to-one caller out of room, with only the other member's file refused", func(t *testing.T) {
		// given
		fx, syncStatus := newSyncFixture(t)
		fx.makeOneToOne(t)
		fx.addFile(t, "file-refused-theirs", filesyncstatus.Limited, 300, otherParticipant())
		expect(syncStatus, anytype, quota(reported(1000, 1000)))

		// when
		got := get(t, fx, false)

		// then: the refusal is theirs; the caller's own shortage is a note of its own
		assert.True(t, got.UpgradeNeeded)
		assert.Equal(t, []string{
			"1 file did not fit in the storage limit and is not backed up. In a one-to-one space each member's files count against their own storage.",
			"The storage limit is reached: new files will not be backed up. Upgrade the plan or delete files to free space.",
		}, got.Notes)
	})

	t.Run("only the other member's files refused in a one-to-one space", func(t *testing.T) {
		// given: the caller has room; the refusal is the other member's
		fx, syncStatus := newSyncFixture(t)
		fx.makeOneToOne(t)
		fx.addFile(t, "file-refused-theirs", filesyncstatus.Limited, 300, otherParticipant())
		expect(syncStatus, anytype, quota(reported(1000, 100)))

		// when
		got := get(t, fx, false)

		// then
		assert.Equal(t, 1, got.NotBackedUpFileCount)
		assert.False(t, got.UpgradeNeeded)
		assert.Equal(t, []string{"1 file did not fit in the storage limit and is not backed up. In a one-to-one space each member's files count against their own storage."}, got.Notes)
	})

	t.Run("waiting files that may not fit, none refused yet: a forecast, no upgrade", func(t *testing.T) {
		// given
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
		fx.addFile(t, "file-queued", filesyncstatus.Queued, 200, myParticipant())
		expect(syncStatus, anytype, quota(reported(1000, 900)))

		// when
		got := get(t, fx, false)

		// then
		assert.False(t, got.UpgradeNeeded)
		assert.Equal(t, []string{"Files waiting to upload may not fit in the storage left. Delete files to free space."}, got.Notes)
	})

	t.Run("a self-hosted network has no plan to upgrade", func(t *testing.T) {
		// given
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
		fx.addFile(t, "file-refused", filesyncstatus.Limited, 300, myParticipant())
		expect(syncStatus, &pb.EventSpaceSyncStatusUpdate{Network: pb.EventSpace_SelfHost}, quota(reported(1000, 1000)))

		// when
		got := get(t, fx, false)

		// then
		assert.Equal(t, v2model.SyncNetworkSelfHosted, got.Network)
		assert.False(t, got.UpgradeNeeded)
		assert.Equal(t, []string{"1 file did not fit in the storage limit and is not backed up. Delete files to free space, or ask the node's administrator to raise the limit."}, got.Notes)
	})

	t.Run("local-only: no storage asked for, nothing listed", func(t *testing.T) {
		// given: SpaceStorage is not expected, so asking for it fails; a
		// local-only account never confirms its objects
		fx, syncStatus := newSyncFixture(t)
		fx.addPendingObject(t, "obj1", domain.ObjectSyncStatusSyncing, domain.SyncErrorNull, 100)
		syncStatus.EXPECT().SpaceSyncStatus(testSpaceId).Return(&pb.EventSpaceSyncStatusUpdate{
			Status: pb.EventSpace_Offline, Network: pb.EventSpace_LocalOnly,
		}, true, nil)
		want := &v2model.SyncStatus{
			SpaceId: testSpaceId,
			Status:  v2model.SyncStatusLocalOnly,
			Network: v2model.SyncNetworkLocalOnly,
			Notes:   []string{"This account is in local-only mode: nothing is backed up to a node."},
		}

		// when
		got := get(t, fx, true)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("a connection not tried yet is connecting, not synced", func(t *testing.T) {
		// given: the app shows the connection as fine until the first check
		fx, syncStatus := newSyncFixture(t)
		syncStatus.EXPECT().SpaceSyncStatus(testSpaceId).Return(&pb.EventSpaceSyncStatusUpdate{
			Status: pb.EventSpace_Synced, Network: pb.EventSpace_Anytype,
		}, false, nil)
		syncStatus.EXPECT().SpaceStorage(testSpaceId).Return(apicore.SpaceStorage{}, false, nil)

		// when
		got := get(t, fx, false)

		// then
		assert.Equal(t, v2model.SyncStatusConnecting, got.Status)
		assert.Equal(t, []string{"This device has not reached the backup node yet, so the status is not confirmed."}, got.Notes)
	})

	t.Run("offline, before the node has reported the quota", func(t *testing.T) {
		// given
		fx, syncStatus := newSyncFixture(t)
		expect(syncStatus, &pb.EventSpaceSyncStatusUpdate{
			Status: pb.EventSpace_Offline, Error: pb.EventSpace_NetworkError, Network: pb.EventSpace_Anytype,
		}, nil)

		// when
		got := get(t, fx, false)

		// then
		assert.Equal(t, v2model.SyncStatusOffline, got.Status)
		assert.Equal(t, v2model.SyncErrorNetwork, got.Error)
		assert.Nil(t, got.Storage)
		assert.Equal(t, []string{"No connection to the backup node. Changes sync once it is back."}, got.Notes)
	})

	t.Run("the app's storage warning is not served", func(t *testing.T) {
		// given: the app raises it from the caller's own account usage, and
		// also when that usage could not be read
		fx, syncStatus := newSyncFixture(t)
		expect(syncStatus, &pb.EventSpaceSyncStatusUpdate{
			Status: pb.EventSpace_Error, Error: pb.EventSpace_StorageLimitExceed, Network: pb.EventSpace_Anytype,
			SyncingObjectsCounter: 2,
		}, nil)

		// when
		got := get(t, fx, false)

		// then
		assert.Equal(t, v2model.SyncStatusSyncing, got.Status)
		assert.Empty(t, got.Error)
		assert.Empty(t, got.Notes)
	})

	t.Run("an app too old for the network", func(t *testing.T) {
		// given
		fx, syncStatus := newSyncFixture(t)
		expect(syncStatus, &pb.EventSpaceSyncStatusUpdate{Status: pb.EventSpace_NetworkNeedsUpdate}, nil)

		// when
		got := get(t, fx, false)

		// then
		assert.Equal(t, v2model.SyncStatusNetworkNeedsUpdate, got.Status)
		assert.Equal(t, []string{"The sync network needs a newer app version. Update the app."}, got.Notes)
	})

	t.Run("an unknown space is a 404 before anything is read", func(t *testing.T) {
		// given: the strict mock fails on any call
		fx, _ := newSyncFixture(t)

		// when
		_, err := fx.GetSyncStatus(context.Background(), "unknownSpace", true, 0, 25)

		// then
		requireV2Status(t, err, http.StatusNotFound)
	})

	t.Run("a status that cannot be counted fails the read, not reads synced", func(t *testing.T) {
		// given
		fx, syncStatus := newSyncFixture(t)
		syncStatus.EXPECT().SpaceSyncStatus(testSpaceId).Return(nil, false, errors.New("store closed"))

		// when
		_, err := fx.GetSyncStatus(context.Background(), testSpaceId, false, 0, 25)

		// then
		require.ErrorContains(t, err, "store closed")
	})

	t.Run("a server without the sync port answers 501", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.GetSyncStatus(context.Background(), testSpaceId, true, 0, 25)

		// then
		requireV2Status(t, err, http.StatusNotImplemented)
	})
}

func TestV2UploadFileStorage(t *testing.T) {
	upload := func(fx *v2Fixture, backup filesyncstatus.Status, size int64, creator string) {
		fx.mwMock.EXPECT().FileUpload(mock.Anything, mock.Anything).Return(&pb.RpcFileUploadResponse{
			ObjectId: "file-new",
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyName.String():             pbtypes.String("a.pdf"),
				bundle.RelationKeySizeInBytes.String():      pbtypes.Int64(size),
				bundle.RelationKeyFileBackupStatus.String(): pbtypes.Int64(int64(backup)),
				bundle.RelationKeyCreator.String():          pbtypes.String(creator),
			}},
			Error: &pb.RpcFileUploadResponseError{Code: pb.RpcFileUploadResponseError_NULL},
		})
	}
	uploadUrl := func(t *testing.T, fx *v2Fixture) *v2model.FileUploadResult {
		got, err := fx.UploadFile(context.Background(), testSpaceId, "", "https://example.org/a.pdf", "", false)
		require.NoError(t, err)
		return got
	}
	expect := func(syncStatus *mock_apicore.MockSyncStatus, network pb.EventSpaceNetwork, q apicore.SpaceStorage, ok bool) {
		syncStatus.EXPECT().Network().Return(network)
		syncStatus.EXPECT().SpaceStorage(testSpaceId).Return(q, ok, nil)
	}
	verdict := func(v bool) *bool { return &v }

	for _, indexed := range []bool{true, false} {
		t.Run(fmt.Sprintf("a file that fits reports the quota with itself counted once (indexed: %v)", indexed), func(t *testing.T) {
			// given: whether or not the store has indexed the new file yet,
			// it is counted once, by its known size
			fx, syncStatus := newSyncFixture(t)
			fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
			fx.addFile(t, "file-earlier", filesyncstatus.Queued, 200, myParticipant())
			if indexed {
				fx.addFile(t, "file-new", filesyncstatus.Queued, 300, myParticipant())
			}
			upload(fx, filesyncstatus.Queued, 300, myParticipant())
			expect(syncStatus, pb.EventSpace_Anytype, reported(1000, 100), true)
			want := &v2model.FileUploadResult{
				Id:        "file-new",
				Name:      "a.pdf",
				Size:      300,
				Storage:   &v2model.Storage{UsedBytes: 100, PendingBytes: 500, LimitBytes: 1000, LeftBytes: 400},
				OverLimit: verdict(false),
			}

			// when
			got := uploadUrl(t, fx)

			// then
			assert.Equal(t, want, got)
		})
	}

	t.Run("the quota is read before the upload, so a fast upload is not counted twice", func(t *testing.T) {
		// given: the uploader confirms the file before the response is built:
		// read afterwards, the usage would hold it and the store still list it
		// as waiting
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
		used := int64(600)
		syncStatus.EXPECT().Network().Return(pb.EventSpace_Anytype)
		syncStatus.EXPECT().SpaceStorage(testSpaceId).RunAndReturn(func(string) (apicore.SpaceStorage, bool, error) {
			return reported(1000, used), true, nil
		}).Once()
		fx.mwMock.EXPECT().FileUpload(mock.Anything, mock.Anything).RunAndReturn(
			func(context.Context, *pb.RpcFileUploadRequest) *pb.RpcFileUploadResponse {
				used += 300 // confirmed already
				fx.addFile(t, "file-new", filesyncstatus.Queued, 300, myParticipant())
				return &pb.RpcFileUploadResponse{
					ObjectId: "file-new",
					Details: &types.Struct{Fields: map[string]*types.Value{
						bundle.RelationKeySizeInBytes.String():      pbtypes.Int64(300),
						bundle.RelationKeyFileBackupStatus.String(): pbtypes.Int64(int64(filesyncstatus.Queued)),
					}},
					Error: &pb.RpcFileUploadResponseError{Code: pb.RpcFileUploadResponseError_NULL},
				}
			})

		// when
		got := uploadUrl(t, fx)

		// then
		assert.Equal(t, verdict(false), got.OverLimit)
		assert.Equal(t, &v2model.Storage{UsedBytes: 600, PendingBytes: 300, LimitBytes: 1000, LeftBytes: 100}, got.Storage)
	})

	t.Run("a file behind others that take the room left is over the limit", func(t *testing.T) {
		// given: 1000 - 600 used - 300 queued before it leaves 100 for a
		// 200-byte file; the node alone would still report 400 free
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
		fx.addFile(t, "file-earlier", filesyncstatus.Queued, 300, myParticipant())
		upload(fx, filesyncstatus.Queued, 200, myParticipant())
		expect(syncStatus, pb.EventSpace_Anytype, reported(1000, 600), true)

		// when
		got := uploadUrl(t, fx)

		// then
		assert.Equal(t, verdict(true), got.OverLimit)
		assert.Equal(t, &v2model.Storage{UsedBytes: 600, PendingBytes: 500, LimitBytes: 1000, LeftBytes: 100}, got.Storage)
		// the node's own figure has room for it: no purchase on a forecast
		assert.Equal(t, "This file does not fit in the storage left, so it stays on this device, not backed up, until there is room. Delete files to free space.", got.Note)
	})

	t.Run("a file the node's own figure has no room for: purchase advised to the payer", func(t *testing.T) {
		// given
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
		upload(fx, filesyncstatus.Queued, 500, myParticipant())
		expect(syncStatus, pb.EventSpace_Anytype, reported(1000, 600), true)

		// when
		got := uploadUrl(t, fx)

		// then
		assert.Equal(t, verdict(true), got.OverLimit)
		assert.Equal(t, "This file does not fit in the storage left, so it stays on this device, not backed up, until there is room. Upgrade the plan or delete files to free space.", got.Note)
	})

	t.Run("a file behind one too large to upload still fits", func(t *testing.T) {
		// given: 400 left; the uploader refuses the queued 500 without taking
		// room, and takes the 100 behind it
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
		fx.addFile(t, "file-too-large", filesyncstatus.Queued, 500, myParticipant())
		upload(fx, filesyncstatus.Queued, 100, myParticipant())
		expect(syncStatus, pb.EventSpace_Anytype, reported(1000, 600), true)

		// when
		got := uploadUrl(t, fx)

		// then
		assert.Equal(t, verdict(false), got.OverLimit)
		assert.Empty(t, got.Note)
	})

	t.Run("bytes the space already backed up need no room", func(t *testing.T) {
		// given: the upload returned an existing, synced file; the space is
		// full, but this file is not waiting for anything
		fx, syncStatus := newSyncFixture(t)
		upload(fx, filesyncstatus.Synced, 300, myParticipant())
		expect(syncStatus, pb.EventSpace_Anytype, reported(1000, 1000), true)

		// when
		got := uploadUrl(t, fx)

		// then
		assert.Equal(t, verdict(false), got.OverLimit)
		assert.Empty(t, got.Note)
		assert.Equal(t, int64(0), got.Storage.PendingBytes)
	})

	t.Run("an existing file refused earlier, with room now, fits", func(t *testing.T) {
		// given: the plan was upgraded since the refusal
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
		upload(fx, filesyncstatus.Limited, 300, myParticipant())
		expect(syncStatus, pb.EventSpace_Anytype, reported(100_000, 1000), true)

		// when
		got := uploadUrl(t, fx)

		// then
		assert.Equal(t, verdict(false), got.OverLimit)
		assert.Empty(t, got.Note)
	})

	t.Run("an existing file refused for the limit, in someone else's space", func(t *testing.T) {
		// given
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Writer)
		upload(fx, filesyncstatus.Limited, 300, myParticipant())
		expect(syncStatus, pb.EventSpace_Anytype, reported(1000, 1000), true)

		// when
		got := uploadUrl(t, fx)

		// then
		assert.Equal(t, verdict(true), got.OverLimit)
		assert.Equal(t, "This file does not fit in the storage left, so it stays on this device, not backed up, until there is room. The limit is the space owner's: delete files you added to free space, or ask the owner for more room.", got.Note)
	})

	t.Run("the other member's existing file in a one-to-one space is not judged", func(t *testing.T) {
		// given: the upload returned the other member's file, billed to them
		fx, syncStatus := newSyncFixture(t)
		fx.makeOneToOne(t)
		upload(fx, filesyncstatus.Limited, 300, otherParticipant())
		expect(syncStatus, pb.EventSpace_Anytype, reported(1000, 1000), true)

		// when
		got := uploadUrl(t, fx)

		// then
		assert.NotNil(t, got.Storage)
		assert.Nil(t, got.OverLimit)
		assert.Empty(t, got.Note)
	})

	t.Run("no quota reported: no storage and no verdict", func(t *testing.T) {
		// given
		fx, syncStatus := newSyncFixture(t)
		upload(fx, filesyncstatus.Queued, 300, myParticipant())
		expect(syncStatus, pb.EventSpace_Anytype, apicore.SpaceStorage{}, false)

		// when
		got := uploadUrl(t, fx)

		// then
		assert.Nil(t, got.Storage)
		assert.Nil(t, got.OverLimit)
	})

	t.Run("a quota that cannot be read leaves the report out, not the upload", func(t *testing.T) {
		// given
		fx, syncStatus := newSyncFixture(t)
		upload(fx, filesyncstatus.Queued, 300, myParticipant())
		syncStatus.EXPECT().Network().Return(pb.EventSpace_Anytype)
		syncStatus.EXPECT().SpaceStorage(testSpaceId).Return(apicore.SpaceStorage{}, false, errors.New("spaceView not found"))

		// when
		got := uploadUrl(t, fx)

		// then
		assert.Equal(t, "file-new", got.Id)
		assert.Nil(t, got.Storage)
		assert.Nil(t, got.OverLimit)
	})

	t.Run("local-only: no storage asked for", func(t *testing.T) {
		// given: SpaceStorage is not expected
		fx, syncStatus := newSyncFixture(t)
		upload(fx, filesyncstatus.Queued, 300, myParticipant())
		syncStatus.EXPECT().Network().Return(pb.EventSpace_LocalOnly)

		// when
		got := uploadUrl(t, fx)

		// then
		assert.Nil(t, got.Storage)
		assert.Nil(t, got.OverLimit)
	})

	t.Run("a file that does not fit, by a limit not known to be the plan's: no purchase advised", func(t *testing.T) {
		// given
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
		upload(fx, filesyncstatus.Queued, 300, myParticipant())
		q := reported(1000, 900)
		q.PlanLimit = false
		expect(syncStatus, pb.EventSpace_Anytype, q, true)

		// when
		got := uploadUrl(t, fx)

		// then
		assert.Equal(t, verdict(true), got.OverLimit)
		assert.Equal(t, "This file does not fit in the storage left, so it stays on this device, not backed up, until there is room. Delete files to free space.", got.Note)
	})

	t.Run("a file that does not fit, by a stale report: what the report said, no remedy", func(t *testing.T) {
		// given: the device has been offline for an hour; the plan may have
		// been upgraded since
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
		upload(fx, filesyncstatus.Queued, 300, myParticipant())
		q := reported(1000, 900)
		q.ReportedAt = time.Now().Add(-time.Hour)
		expect(syncStatus, pb.EventSpace_Anytype, q, true)

		// when
		got := uploadUrl(t, fx)

		// then
		assert.Equal(t, verdict(true), got.OverLimit)
		assert.Equal(t, "By the backup node's last report, this file does not fit in the storage left, so it stays on this device, not backed up, until there is room.", got.Note)
	})

	t.Run("re-uploading bytes the node already confirmed needs no room", func(t *testing.T) {
		// given: the space is full; the upload returns an existing file the
		// node confirmed and the store still reads as syncing
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
		fx.addFile(t, "file-new", filesyncstatus.Syncing, 300, myParticipant())
		upload(fx, filesyncstatus.Syncing, 300, myParticipant())
		q := reported(1000, 1000)
		q.Confirmed = map[string]apicore.ConfirmedUpload{"file-new": {Bytes: 300, InReport: true}}
		expect(syncStatus, pb.EventSpace_Anytype, q, true)

		// when
		got := uploadUrl(t, fx)

		// then
		assert.Equal(t, verdict(false), got.OverLimit)
		assert.Empty(t, got.Note)
	})

	t.Run("an archived file still waiting takes room", func(t *testing.T) {
		// given: archiving does not stop an upload
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
		fx.addFile(t, "file-archived", filesyncstatus.Queued, 300, myParticipant())
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:               domain.String("file-archived"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_file)),
			bundle.RelationKeyFileBackupStatus: domain.Int64(int64(filesyncstatus.Queued)),
			bundle.RelationKeySizeInBytes:      domain.Int64(300),
			bundle.RelationKeyCreator:          domain.String(myParticipant()),
			bundle.RelationKeyIsArchived:       domain.Bool(true),
		}})
		upload(fx, filesyncstatus.Queued, 200, myParticipant())
		expect(syncStatus, pb.EventSpace_Anytype, reported(1000, 600), true)

		// when
		got := uploadUrl(t, fx)

		// then
		assert.Equal(t, verdict(true), got.OverLimit)
		assert.Equal(t, int64(500), got.Storage.PendingBytes)
	})

	t.Run("an upload in flight takes the room before a later one", func(t *testing.T) {
		// given: a 300-byte multipart upload is still in its RPC when a
		// 200-byte url upload starts; 400 is free. Both read the store before
		// either file is in it.
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
		staged := filepath.Join(t.TempDir(), "staged.bin")
		require.NoError(t, os.WriteFile(staged, make([]byte, 300), 0o600))
		syncStatus.EXPECT().Network().Return(pb.EventSpace_Anytype)
		syncStatus.EXPECT().SpaceStorage(testSpaceId).Return(reported(1000, 600), true, nil)
		inFirst, releaseFirst := make(chan struct{}), make(chan struct{})
		fx.mwMock.EXPECT().FileUpload(mock.Anything, mock.MatchedBy(func(req *pb.RpcFileUploadRequest) bool {
			return req.LocalPath == staged
		})).RunAndReturn(func(context.Context, *pb.RpcFileUploadRequest) *pb.RpcFileUploadResponse {
			close(inFirst)
			<-releaseFirst
			return &pb.RpcFileUploadResponse{ObjectId: "file-first", Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeySizeInBytes.String():      pbtypes.Int64(300),
				bundle.RelationKeyFileBackupStatus.String(): pbtypes.Int64(int64(filesyncstatus.Queued)),
			}}, Error: &pb.RpcFileUploadResponseError{Code: pb.RpcFileUploadResponseError_NULL}}
		})
		fx.mwMock.EXPECT().FileUpload(mock.Anything, mock.MatchedBy(func(req *pb.RpcFileUploadRequest) bool {
			return req.Url != ""
		})).Return(&pb.RpcFileUploadResponse{ObjectId: "file-second", Details: &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeySizeInBytes.String():      pbtypes.Int64(200),
			bundle.RelationKeyFileBackupStatus.String(): pbtypes.Int64(int64(filesyncstatus.Queued)),
		}}, Error: &pb.RpcFileUploadResponseError{Code: pb.RpcFileUploadResponseError_NULL}})
		first := make(chan *v2model.FileUploadResult, 1)
		go func() {
			got, err := fx.UploadFile(context.Background(), testSpaceId, staged, "", "", false)
			assert.NoError(t, err)
			first <- got
		}()
		select {
		case <-inFirst:
		case <-time.After(10 * time.Second):
			t.Fatal("the first upload never reached its RPC")
		}

		// when
		second := uploadUrl(t, fx)
		close(releaseFirst)

		// then: the first fits, the second is behind it and does not
		assert.Equal(t, verdict(true), second.OverLimit)
		assert.Equal(t, verdict(false), (<-first).OverLimit)
	})

	t.Run("an earlier upload of the same bytes is not counted twice", func(t *testing.T) {
		// given: two uploads of one 300-byte file; 500 is free. The first is
		// in its RPC when the second starts, and resolves to the same object
		// before the second is judged.
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
		staged := filepath.Join(t.TempDir(), "staged.bin")
		require.NoError(t, os.WriteFile(staged, make([]byte, 300), 0o600))
		syncStatus.EXPECT().Network().Return(pb.EventSpace_Anytype)
		syncStatus.EXPECT().SpaceStorage(testSpaceId).Return(reported(1000, 500), true, nil)
		same := &pb.RpcFileUploadResponse{ObjectId: "file-same", Details: &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeySizeInBytes.String():      pbtypes.Int64(300),
			bundle.RelationKeyFileBackupStatus.String(): pbtypes.Int64(int64(filesyncstatus.Queued)),
		}}, Error: &pb.RpcFileUploadResponseError{Code: pb.RpcFileUploadResponseError_NULL}}
		var calls atomic.Int32
		inFirst, releaseFirst, releaseSecond := make(chan struct{}), make(chan struct{}), make(chan struct{})
		fx.mwMock.EXPECT().FileUpload(mock.Anything, mock.Anything).RunAndReturn(func(context.Context, *pb.RpcFileUploadRequest) *pb.RpcFileUploadResponse {
			if calls.Add(1) == 1 {
				close(inFirst)
				<-releaseFirst
			} else {
				<-releaseSecond
			}
			return same
		})
		first := make(chan *v2model.FileUploadResult, 1)
		go func() {
			got, err := fx.UploadFile(context.Background(), testSpaceId, staged, "", "", false)
			assert.NoError(t, err)
			first <- got
		}()
		<-inFirst
		second := make(chan *v2model.FileUploadResult, 1)
		go func() {
			got, err := fx.UploadFile(context.Background(), testSpaceId, staged, "", "", false)
			assert.NoError(t, err)
			second <- got
		}()
		require.Eventually(t, func() bool { return calls.Load() == 2 }, 10*time.Second, time.Millisecond)

		// when: the first finishes, then the second
		close(releaseFirst)
		<-first
		close(releaseSecond)

		// then: one file of 300, which fits
		got := <-second
		assert.Equal(t, verdict(false), got.OverLimit)
		assert.Equal(t, int64(300), got.Storage.PendingBytes)
	})

	t.Run("an earlier url upload of unknown size leaves the verdict out, unless it cannot matter", func(t *testing.T) {
		for name, tc := range map[string]struct {
			size int64
			want *bool
		}{
			"the file fits without it: it may yet take the room": {100, nil},
			"the file does not fit even without it":              {500, verdict(true)},
		} {
			t.Run(name, func(t *testing.T) {
				// given: a url upload is still fetching; 400 is free
				fx, syncStatus := newSyncFixture(t)
				fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
				staged := filepath.Join(t.TempDir(), "staged.bin")
				require.NoError(t, os.WriteFile(staged, make([]byte, tc.size), 0o600))
				syncStatus.EXPECT().Network().Return(pb.EventSpace_Anytype)
				syncStatus.EXPECT().SpaceStorage(testSpaceId).Return(reported(1000, 600), true, nil)
				inFetch, releaseFetch := make(chan struct{}), make(chan struct{})
				t.Cleanup(func() {
					select {
					case <-releaseFetch:
					default:
						close(releaseFetch)
					}
				})
				fx.mwMock.EXPECT().FileUpload(mock.Anything, mock.MatchedBy(func(req *pb.RpcFileUploadRequest) bool {
					return req.Url != ""
				})).RunAndReturn(func(context.Context, *pb.RpcFileUploadRequest) *pb.RpcFileUploadResponse {
					close(inFetch)
					<-releaseFetch
					return &pb.RpcFileUploadResponse{ObjectId: "file-fetched", Error: &pb.RpcFileUploadResponseError{Code: pb.RpcFileUploadResponseError_NULL}}
				})
				fx.mwMock.EXPECT().FileUpload(mock.Anything, mock.MatchedBy(func(req *pb.RpcFileUploadRequest) bool {
					return req.LocalPath == staged
				})).Return(&pb.RpcFileUploadResponse{ObjectId: "file-staged", Details: &types.Struct{Fields: map[string]*types.Value{
					bundle.RelationKeySizeInBytes.String():      pbtypes.Int64(tc.size),
					bundle.RelationKeyFileBackupStatus.String(): pbtypes.Int64(int64(filesyncstatus.Queued)),
				}}, Error: &pb.RpcFileUploadResponseError{Code: pb.RpcFileUploadResponseError_NULL}})
				fetched := make(chan struct{})
				go func() {
					defer close(fetched)
					_, err := fx.UploadFile(context.Background(), testSpaceId, "", "https://example.org/big.bin", "", false)
					assert.NoError(t, err)
				}()
				<-inFetch

				// when
				got, err := fx.UploadFile(context.Background(), testSpaceId, staged, "", "", false)

				// then
				require.NoError(t, err)
				assert.Equal(t, tc.want, got.OverLimit)
				close(releaseFetch)
				<-fetched
			})
		}
	})

	t.Run("a multipart dry run says whether the file would fit, uploading nothing", func(t *testing.T) {
		// given: FileUpload is not expected, so uploading fails the test
		fx, syncStatus := newSyncFixture(t)
		fx.setOwnPermissions(t, model.ParticipantPermissions_Owner)
		staged := filepath.Join(t.TempDir(), "staged.bin")
		require.NoError(t, os.WriteFile(staged, make([]byte, 300), 0o600))
		expect(syncStatus, pb.EventSpace_Anytype, reported(1000, 800), true)
		want := &v2model.FileUploadResult{
			DryRun:    true,
			Storage:   &v2model.Storage{UsedBytes: 800, PendingBytes: 300, LimitBytes: 1000, LeftBytes: 200},
			OverLimit: verdict(true),
			Note:      "This file would not fit in the storage left, so it would stay on this device, not backed up, until there is room. Upgrade the plan or delete files to free space.",
		}

		// when
		got, err := fx.UploadFile(context.Background(), testSpaceId, staged, "", "", true)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("a dry run's result serializes without an id", func(t *testing.T) {
		// given
		fx, syncStatus := newSyncFixture(t)
		expect(syncStatus, pb.EventSpace_Anytype, reported(1000, 0), true)

		// when
		got, err := fx.UploadFile(context.Background(), testSpaceId, "", "https://example.org/a.pdf", "", true)
		require.NoError(t, err)
		raw, err := json.Marshal(got)

		// then
		require.NoError(t, err)
		assert.NotContains(t, string(raw), `"id"`)
	})

	t.Run("a url dry run reports the quota, with no verdict", func(t *testing.T) {
		// given: the size is unknown until the url is fetched
		fx, syncStatus := newSyncFixture(t)
		fx.addFile(t, "file-earlier", filesyncstatus.Queued, 100, myParticipant())
		expect(syncStatus, pb.EventSpace_Anytype, reported(1000, 800), true)
		want := &v2model.FileUploadResult{
			DryRun:  true,
			Storage: &v2model.Storage{UsedBytes: 800, PendingBytes: 100, LimitBytes: 1000, LeftBytes: 100},
		}

		// when
		got, err := fx.UploadFile(context.Background(), testSpaceId, "", "https://example.org/a.pdf", "", true)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})
}
