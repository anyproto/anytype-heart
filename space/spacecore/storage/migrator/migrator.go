package migrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage/migration"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/anystorehelper"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceresolverstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space/spacecore/oldstorage"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/migratorfinisher"
	"github.com/anyproto/anytype-heart/util/freespace"
)

type NotEnoughFreeSpaceError struct {
	Free     uint64
	Required uint64
}

func (e *NotEnoughFreeSpaceError) Error() string {
	if e.Required == 0 {
		return fmt.Sprintf("not enough free space: %d", e.Free)
	}
	return fmt.Sprintf("Not enough free space: %d, required: %d", e.Free, e.Required)
}

var log = logging.Logger(CName)

const CName = "client.storage.migration"

type migrator struct {
	oldStorage      oldstorage.ClientStorage
	newStorage      storage.ClientStorage
	process         process.Service
	path            string
	objectStorePath string
	finisher        migratorfinisher.Service

	anyStoreConfig *anystore.Config
}

type pathProvider interface {
	GetNewSpaceStorePath() string
	GetOldSpaceStorePath() string
	GetRepoPath() string
	GetAnyStoreConfig() *anystore.Config
}

func New() app.ComponentRunnable {
	return &migrator{}
}

func (m *migrator) Init(a *app.App) (err error) {
	cfg := a.MustComponent("config").(pathProvider)
	m.path = cfg.GetNewSpaceStorePath()
	m.objectStorePath = filepath.Join(cfg.GetRepoPath(), "objectstore")
	m.oldStorage = app.MustComponent[oldstorage.ClientStorage](a)
	m.newStorage = app.MustComponent[storage.ClientStorage](a)
	m.process = app.MustComponent[process.Service](a)
	m.finisher = app.MustComponent[migratorfinisher.Service](a)
	m.anyStoreConfig = cfg.GetAnyStoreConfig()
	return nil
}

func (m *migrator) Name() (name string) {
	return CName
}

func isDiskFull(err error) bool {
	if err == nil {
		return false
	}
	// From sqlite
	if strings.Contains(err.Error(), "disk is full") {
		return true
	}

	// For unix systems
	if errors.Is(err, syscall.ENOSPC) {
		return true
	}
	// For windows
	var syscallErrno syscall.Errno
	if errors.As(err, &syscallErrno) {
		// See https://pkg.go.dev/golang.org/x/sys/windows
		// ERROR_DISK_FULL syscall.Errno = 112
		return syscallErrno == 112
	}
	return false
}

func (m *migrator) Run(ctx context.Context) (err error) {
	oldSize, err := m.oldStorage.EstimateSize()
	if err != nil {
		return fmt.Errorf("estimate size: %w", err)
	}
	free, err := freespace.GetFreeDiskSpace(m.path)
	if err != nil {
		return fmt.Errorf("get free disk space: %w", err)
	}
	requiredDiskSpace := oldSize * 15 / 10
	if requiredDiskSpace > free {
		return &NotEnoughFreeSpaceError{
			Free:     free,
			Required: requiredDiskSpace,
		}
	}

	err = m.run(ctx)
	if err != nil {
		if isDiskFull(err) {
			return &NotEnoughFreeSpaceError{
				Free:     free,
				Required: requiredDiskSpace,
			}
		}
		return err
	}
	return nil
}

func (m *migrator) closeNewStorage(ctx context.Context, newStorage spacestorage.SpaceStorage) (err error) {
	if newStorage == nil {
		return nil
	}
	store := newStorage.AnyStore()
	err = m.newStorage.Close(ctx)
	if err != nil {
		return fmt.Errorf("close new storage: %w", err)
	}
	err = store.Close()
	if err != nil {
		return fmt.Errorf("close new storage any store: %w", err)
	}
	return nil
}

func (m *migrator) run(ctx context.Context) (err error) {
	progress := process.NewProgress(&pb.ModelProcessMessageOfMigration{Migration: &pb.ModelProcessMigration{}})
	progress.SetProgressMessage("Migrating spaces")
	err = m.process.Add(progress)
	if err != nil {
		return err
	}
	defer func() {
		progress.Finish(err)
	}()
	migrator := migration.NewSpaceMigrator(m.oldStorage, m.newStorage, 40, m.path, func(newStorage spacestorage.SpaceStorage, id, rootPath string) error {
		err := m.closeNewStorage(ctx, newStorage)
		if err != nil {
			log.Error("failed to close new storage", zap.String("spaceId", id), zap.Error(err))
		}
		return os.RemoveAll(filepath.Join(rootPath, id))
	})
	allIds, err := m.oldStorage.AllSpaceIds()
	if err != nil {
		return err
	}
	var (
		total    int64
		totalMap = make(map[string]int64)
	)
	for _, id := range allIds {
		store, err := m.oldStorage.WaitSpaceStorage(ctx, id)
		if err != nil {
			return err
		}
		storedIds, err := store.StoredIds()
		if err != nil {
			return err
		}
		total += int64(len(storedIds))
		totalMap[id] = int64(len(storedIds))
		err = store.Close(ctx)
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	progress.SetTotal(total)
	for _, id := range allIds {
		err := migrator.MigrateId(ctx, id, progress)
		if err != nil {
			if errors.Is(err, migration.ErrAlreadyMigrated) {
				progress.AddDone(totalMap[id])
				continue
			}
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	err = m.doObjectStoreDb(ctx, func(db anystore.DB) error {
		resolverStore, err := spaceresolverstore.New(ctx, db)
		if err != nil {
			return fmt.Errorf("new resolver store: %w", err)
		}

		for _, spaceId := range allIds {
			objectIds, err := m.oldStorage.GetBoundObjectIds(spaceId)
			if err != nil {
				return fmt.Errorf("get bound object ids: %w", err)
			}

			for _, objectId := range objectIds {
				err = resolverStore.BindSpaceId(spaceId, objectId)
				if err != nil {
					return fmt.Errorf("bind space id: %w", err)
				}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("migrate space id bindings: %w", err)
	}

	// TODO Maybe add some condition?
	m.finisher.SetMigrationDone()

	return nil
}

// nolint:unused
func (m *migrator) verify(ctx context.Context, fast bool) ([]*verificationReport, error) {
	var reports []*verificationReport
	err := m.doObjectStoreDb(ctx, func(db anystore.DB) error {
		resolverStore, err := spaceresolverstore.New(ctx, db)
		if err != nil {
			return fmt.Errorf("new resolver store: %w", err)
		}

		v := &verifier{
			fast:          fast,
			oldStorage:    m.oldStorage,
			newStorage:    m.newStorage,
			resolverStore: resolverStore,
		}
		reports, err = v.verify(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return reports, nil
}

func (m *migrator) doObjectStoreDb(ctx context.Context, proc func(db anystore.DB) error) error {
	err := ensureDirExists(m.objectStorePath)
	if err != nil {
		return fmt.Errorf("ensure dir exists: %w", err)
	}

	store, lockRemove, err := anystorehelper.OpenDatabaseWithLockCheck(ctx, filepath.Join(m.objectStorePath, "objects.db"), m.anyStoreConfig)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	err = proc(store)

	return errors.Join(err, store.Close(), lockRemove())
}

func ensureDirExists(dir string) error {
	_, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		err = os.MkdirAll(dir, 0700)
		if err != nil {
			return fmt.Errorf("create db dir: %w", err)
		}
	}
	return nil
}

func (m *migrator) Close(ctx context.Context) (err error) {
	return nil
}
