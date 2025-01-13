package migrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	oldstorage2 "github.com/anyproto/any-sync/commonspace/spacestorage/oldstorage"
	"golang.org/x/exp/slices"

	"github.com/anyproto/any-sync/commonspace/spacestorage/migration"

	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/anystorehelper"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceresolverstore"
	"github.com/anyproto/anytype-heart/space/spacecore/oldstorage"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/migratorfinisher"
)

const CName = "client.storage.migration"

type migrator struct {
	oldStorage      oldstorage.ClientStorage
	newStorage      storage.ClientStorage
	process         process.Service
	path            string
	oldPath         string
	objectStorePath string
	finisher        migratorfinisher.Service
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
	m.oldPath = cfg.GetOldSpaceStorePath()
	m.objectStorePath = filepath.Join(cfg.GetRepoPath(), "objectstore")
	m.oldStorage = app.MustComponent[oldstorage.ClientStorage](a)
	m.newStorage = app.MustComponent[storage.ClientStorage](a)
	m.process = app.MustComponent[process.Service](a)
	m.finisher = app.MustComponent[migratorfinisher.Service](a)
	return nil
}

func (m *migrator) Name() (name string) {
	return CName
}

func (m *migrator) Run(ctx context.Context) (err error) {
	progress := process.NewProgress(&pb.ModelProcessMessageOfMigration{Migration: &pb.ModelProcessMigration{}})
	progress.SetProgressMessage("Migrating spaces")
	err = m.process.Add(progress)
	if err != nil {
		return err
	}
	defer func() {
		progress.Finish(err)
	}()
	migrator := migration.NewSpaceMigrator(m.oldStorage, m.newStorage, 40, m.path)
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

func (m *migrator) verify(ctx context.Context, fast bool) error {
	allSpaceIds, err := m.oldStorage.AllSpaceIds()
	if err != nil {
		return fmt.Errorf("list all space ids: %w", err)
	}
	for _, spaceId := range allSpaceIds {
		err := m.verifySpace(ctx, spaceId, fast)
		if err != nil {
			return fmt.Errorf("verify space: %w", err)
		}
	}
	return nil
}

func (m *migrator) verifySpace(ctx context.Context, spaceId string, fast bool) error {
	oldStore, err := m.oldStorage.WaitSpaceStorage(ctx, spaceId)
	if err != nil {
		return fmt.Errorf("open old store: %w", err)
	}

	newStore, err := m.newStorage.WaitSpaceStorage(ctx, spaceId)
	if err != nil {
		return fmt.Errorf("open new store: %w", err)
	}
	newHeadStorage := newStore.HeadStorage()

	storedIds, err := oldStore.StoredIds()
	if err != nil {
		return err
	}

	var bytesCompared int
	for _, treeId := range storedIds {

		entry, err := newHeadStorage.GetEntry(ctx, treeId)
		if err != nil {
			return fmt.Errorf("get heads entry: %w", err)
		}
		fmt.Println(entry.Heads)

		oldTreeStorage, err := oldStore.TreeStorage(treeId)
		if err != nil {
			return fmt.Errorf("open old tree storage: %w", err)
		}
		oldHeads, err := oldTreeStorage.Heads()
		if err != nil {
			return fmt.Errorf("open old heads storage: %w", err)
		}
		if !slices.Equal(oldHeads, entry.Heads) {
			return fmt.Errorf("old heads does not match tree storage")
		}

		newTreeStorage, err := newStore.TreeStorage(ctx, treeId)
		if err != nil {
			return fmt.Errorf("open new tree storage: %w", err)
		}

		if fast {
			err = m.verifyTreeFast(ctx, oldTreeStorage, newTreeStorage)
			if err != nil {
				return fmt.Errorf("verify tree fast: %w", err)
			}
		}

	}

	fmt.Println(spaceId, "bytes compared", bytesCompared)

	err = oldStore.Close(ctx)
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return nil
}

func (m *migrator) verifyTreeFast(ctx context.Context, oldTreeStorage oldstorage2.TreeStorage, newTreeStorage objecttree.Storage) error {
	oldChangeIds, err := oldTreeStorage.GetAllChangeIds()
	if err != nil {
		return fmt.Errorf("get old change ids: %w", err)
	}

	if len(oldChangeIds) == 0 {
		return fmt.Errorf("old change ids is empty")
	}
	for _, oldChangeId := range oldChangeIds {
		ok, err := newTreeStorage.Has(ctx, oldChangeId)
		if err != nil {
			return fmt.Errorf("get old change id: %w", err)
		}
		if !ok {
			return fmt.Errorf("old change id does not exist")
		}
	}
	return nil
}

func (m *migrator) doObjectStoreDb(ctx context.Context, proc func(db anystore.DB) error) error {
	// TODO cfg
	cfg := &anystore.Config{}

	err := ensureDirExists(m.objectStorePath)
	if err != nil {
		return fmt.Errorf("ensure dir exists: %w", err)
	}

	store, lockRemove, err := anystorehelper.OpenDatabaseWithLockCheck(ctx, filepath.Join(m.objectStorePath, "objects.db"), cfg)
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
