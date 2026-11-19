package spaceoffloader

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/deletioncontroller/mock_deletioncontroller"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies/mock_dependencies"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus/mock_spacestatus"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/mock_storage"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

type fileOffloaderStub struct {
	offloaded    []string
	ctxErrs      []error
	hasDeadlines []bool
	record       func(string)
}

func (f *fileOffloaderStub) FileSpaceOffload(ctx context.Context, spaceId string, _ bool) (int, uint64, error) {
	f.offloaded = append(f.offloaded, spaceId)
	f.ctxErrs = append(f.ctxErrs, ctx.Err())
	_, hasDeadline := ctx.Deadline()
	f.hasDeadlines = append(f.hasDeadlines, hasDeadline)
	if f.record != nil {
		f.record("offload files")
	}
	return 0, 0, nil
}

// backups lists two backups of space1's corrupted storage the storage service made, one the user
// renamed to keep, and one of space2's
var backups = []storage.CorruptedBackup{
	{SpaceId: "space1", BackupPath: "/spacestore/space1_backup1"},
	{SpaceId: "space2", BackupPath: "/spacestore/space2_backup1"},
	{SpaceId: "space1", BackupPath: "/spacestore/space1_backup_keep"},
	{SpaceId: "space1", BackupPath: "/spacestore/space1_backup01"}, // not as the writer spells it
	{SpaceId: "space1", BackupPath: "/spacestore/space1_backup2"},
}

func TestOffloadRemovesTheStorageBackupsOfTheSpace(t *testing.T) {
	// given
	ctx := context.Background()
	clientStorage := mock_storage.NewMockClientStorage(t)
	indexer := mock_dependencies.NewMockSpaceIndexer(t)
	var calls []string
	record := func(call string) { calls = append(calls, call) }
	files := &fileOffloaderStub{record: record}
	o := &spaceOffloader{storageService: clientStorage, indexer: indexer, fileOffloader: files}
	var storageDeadline bool
	clientStorage.EXPECT().DeleteSpaceStorage(mock.Anything, "space1").RunAndReturn(func(ctx context.Context, _ string) error {
		_, storageDeadline = ctx.Deadline()
		record("delete storage")
		return nil
	})
	clientStorage.EXPECT().ListCorruptedBackups().Return(backups)
	clientStorage.EXPECT().DeleteBackup(mock.Anything).RunAndReturn(func(path string) error {
		record("delete " + path)
		return nil
	})
	indexer.EXPECT().RemoveIndexes("space1").RunAndReturn(func(string) error {
		record("remove indexes")
		return nil
	})

	// when
	err := o.offload(ctx, "space1")

	// then: the space's generated backups go after its storage; the renamed one and the other
	// space's stay; the files are offloaded with a live context, the indexes go last
	require.NoError(t, err)
	assert.Equal(t, []string{
		"delete storage",
		"delete /spacestore/space1_backup1",
		"delete /spacestore/space1_backup2",
		"offload files",
		"remove indexes",
	}, calls)
	assert.Equal(t, []string{"space1"}, files.offloaded)
	assert.Equal(t, []error{nil}, files.ctxErrs)
	// the timeout bounds the storage lock wait only, not the file cleanup
	assert.True(t, storageDeadline)
	assert.Equal(t, []bool{false}, files.hasDeadlines)
}

func TestOffloadGoesOnWhenABackupCannotBeDeleted(t *testing.T) {
	// given: a backup that cannot be deleted (no permission, say)
	clientStorage := mock_storage.NewMockClientStorage(t)
	indexer := mock_dependencies.NewMockSpaceIndexer(t)
	files := &fileOffloaderStub{}
	o := &spaceOffloader{storageService: clientStorage, indexer: indexer, fileOffloader: files}
	clientStorage.EXPECT().DeleteSpaceStorage(mock.Anything, "space1").Return(nil)
	clientStorage.EXPECT().ListCorruptedBackups().Return(backups)
	clientStorage.EXPECT().DeleteBackup("/spacestore/space1_backup1").Return(errors.New("permission denied"))
	clientStorage.EXPECT().DeleteBackup("/spacestore/space1_backup2").Return(nil)
	indexer.EXPECT().RemoveIndexes("space1").Return(nil)

	// when
	err := o.offload(context.Background(), "space1")

	// then: the offload completes; the backup is tried again at the next start
	require.NoError(t, err)
	assert.Equal(t, []string{"space1"}, files.offloaded)
}

func TestRunOfAnOffloadedSpaceRemovesItsStorageBackups(t *testing.T) {
	// given: a space a version that kept the backups offloaded
	clientStorage := mock_storage.NewMockClientStorage(t)
	status := mock_spacestatus.NewMockSpaceStatus(t)
	deletion := mock_deletioncontroller.NewMockDeletionController(t)
	o := &spaceOffloader{storageService: clientStorage, status: status, delController: deletion}
	status.EXPECT().GetLocalStatus().Return(spaceinfo.LocalStatusMissing)
	status.EXPECT().SpaceId().Return("space1")
	deletion.EXPECT().AddSpaceToDelete("space1").Return()
	clientStorage.EXPECT().ListCorruptedBackups().Return(backups)
	var deleted []string
	clientStorage.EXPECT().DeleteBackup(mock.Anything).RunAndReturn(func(path string) error {
		deleted = append(deleted, path)
		return nil
	})

	// when
	require.NoError(t, o.Run(context.Background()))

	// then
	assert.True(t, o.offloaded.Load())
	assert.Equal(t, []string{"/spacestore/space1_backup1", "/spacestore/space1_backup2"}, deleted)
}
