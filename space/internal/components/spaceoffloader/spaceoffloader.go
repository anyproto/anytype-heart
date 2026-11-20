package spaceoffloader

/*
AI generated

Name: Local Space Data Offloader
Scope: space

## Responsibility
- Deletes local space storage, files, and indexes when space is being removed

## Background Tasks
- offloadRetry: Retries offload operation every 20s until success or context cancellation
*/

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/deletioncontroller"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

const CName = "client.components.spaceoffloader"

var log = logger.NewNamed(CName)

const deleteStorageLockTimeout = time.Second * 10

type SpaceOffloader interface {
	app.ComponentRunnable
	WaitOffload(ctx context.Context) (err error)
}

func New() SpaceOffloader {
	return &spaceOffloader{}
}

type spaceOffloader struct {
	status         spacestatus.SpaceStatus
	offloading     *offloadingSpace
	fileOffloader  dependencies.FileOffloader
	storageService storage.ClientStorage
	indexer        dependencies.SpaceIndexer
	delController  deletioncontroller.DeletionController
	ctx            context.Context
	cancel         context.CancelFunc
	offloaded      atomic.Bool
}

func (o *spaceOffloader) Init(a *app.App) (err error) {
	o.status = app.MustComponent[spacestatus.SpaceStatus](a)
	o.fileOffloader = app.MustComponent[dependencies.FileOffloader](a)
	o.storageService = app.MustComponent[storage.ClientStorage](a)
	o.indexer = app.MustComponent[dependencies.SpaceIndexer](a)
	o.delController = app.MustComponent[deletioncontroller.DeletionController](a)
	o.ctx, o.cancel = context.WithCancel(context.Background())
	return nil
}

func (o *spaceOffloader) Name() (name string) {
	return CName
}

func (o *spaceOffloader) Run(ctx context.Context) (err error) {
	localStatus := o.status.GetLocalStatus()
	o.delController.AddSpaceToDelete(o.status.SpaceId())
	if localStatus == spaceinfo.LocalStatusMissing {
		// a version that offloaded the space kept the backups of its corrupted storage
		if err := removeStorageBackups(o.storageService, o.status.SpaceId()); err != nil {
			log.Warn("remove storage backups of an offloaded space", zap.String("spaceId", o.status.SpaceId()), zap.Error(err))
		}
		o.offloaded.Store(true)
		return nil
	}
	o.offloading = newOffloadingSpace(o.ctx, o.status.SpaceId(), o)
	return nil
}

func (o *spaceOffloader) Close(ctx context.Context) (err error) {
	o.cancel()
	ol := o.offloading
	if ol != nil {
		<-ol.loadCh
	}
	return nil
}

func (o *spaceOffloader) CanTransition(next mode.Mode) bool {
	return false
}

func (o *spaceOffloader) onOffload(id string, offloadErr error) {
	if offloadErr != nil {
		log.Warn("offload error", zap.Error(offloadErr), zap.String("spaceId", id))
		return
	}
	if err := o.status.SetLocalStatus(spaceinfo.LocalStatusMissing); err != nil {
		log.Debug("set status error", zap.Error(err), zap.String("spaceId", id))
	}
	o.offloaded.Store(true)
}

func (o *spaceOffloader) WaitOffload(ctx context.Context) (err error) {
	if o.offloaded.Load() {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-o.offloading.loadCh:
		return o.offloading.loadErr
	}
}

func (o *spaceOffloader) offload(ctx context.Context, id string) (err error) {
	// the timeout bounds the wait for the storage's lock only: the file cleanup below gets ctx
	lockCtx, cancel := context.WithTimeout(ctx, deleteStorageLockTimeout)
	err = o.storageService.DeleteSpaceStorage(lockCtx, id)
	cancel()
	if err != nil {
		return
	}
	// The space is left or removed: a backup of its storage, moved aside when it was found corrupted
	// for the space to be fetched again, is of no use any more. A backup that can't be deleted holds
	// back nothing else; the start after the offload tries again.
	if err = removeStorageBackups(o.storageService, id); err != nil {
		log.Warn("remove storage backups of an offloaded space", zap.String("spaceId", id), zap.Error(err))
	}
	_, _, err = o.fileOffloader.FileSpaceOffload(ctx, id, true)
	if err != nil {
		return err
	}
	return o.indexer.RemoveIndexes(id)
}

// storageBackups lists and deletes the backups of corrupted space storage (storage.ClientStorage).
type storageBackups interface {
	ListCorruptedBackups() []storage.CorruptedBackup
	DeleteBackup(backupPath string) error
}

// removeStorageBackups deletes every backup of the space's storage the storage service made: a
// directory named <spaceId>_backup<timestamp>. Another directory that only looks like one (a backup
// the user renamed to keep it, say) stays.
func removeStorageBackups(backups storageBackups, spaceId string) error {
	var errs []error
	for _, backup := range backups.ListCorruptedBackups() {
		if backup.SpaceId != spaceId || !isGeneratedBackupName(filepath.Base(backup.BackupPath), spaceId) {
			continue
		}
		if err := backups.DeleteBackup(backup.BackupPath); err != nil {
			errs = append(errs, fmt.Errorf("delete backup %s: %w", backup.BackupPath, err))
		}
	}
	return errors.Join(errs...)
}

// isGeneratedBackupName reports whether name is <spaceId>_backup<unix seconds> spelled as the storage
// service writes it, the seconds in canonical decimal (no leading zeros).
func isGeneratedBackupName(name, spaceId string) bool {
	timestamp, ok := strings.CutPrefix(name, spaceId+"_backup")
	if !ok {
		return false
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	return err == nil && strconv.FormatInt(seconds, 10) == timestamp
}
