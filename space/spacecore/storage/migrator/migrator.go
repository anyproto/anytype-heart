package migrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/any-sync/commonspace/spacestorage/migration"

	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/anystorehelper"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceresolverstore"
	"github.com/anyproto/anytype-heart/space/spacecore/oldstorage"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
)

const CName = "client.storage.migration"

const (
	migratedName      = "space_store_migrated"
	objectStoreFolder = "objectstore"
	crdtDb            = "crdt"
)

type migrator struct {
	storage         oldstorage.ClientStorage
	newStorage      storage.ClientStorage
	process         process.Service
	path            string
	oldPath         string
	objectStorePath string
}

type pathProvider interface {
	GetNewSpaceStorePath() string
	GetOldSpaceStorePath() string
	GetObjectStorePath() string
}

func New() app.ComponentRunnable {
	return &migrator{}
}

func (m *migrator) Init(a *app.App) (err error) {
	cfg := a.MustComponent("config").(pathProvider)
	m.path = cfg.GetNewSpaceStorePath()
	m.oldPath = cfg.GetOldSpaceStorePath()
	m.objectStorePath = cfg.GetObjectStorePath()
	m.storage = app.MustComponent[oldstorage.ClientStorage](a)
	m.newStorage = app.MustComponent[storage.ClientStorage](a)
	m.process = app.MustComponent[process.Service](a)
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
	migrator := migration.NewSpaceMigrator(m.storage, m.newStorage, 40, m.path)
	allIds, err := m.storage.AllSpaceIds()
	if err != nil {
		return err
	}
	var (
		total    int64
		totalMap = make(map[string]int64)
	)
	for _, id := range allIds {
		store, err := m.storage.WaitSpaceStorage(ctx, id)
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
		st, err := migrator.MigrateId(ctx, id, progress)
		if err != nil {
			if errors.Is(err, migration.ErrAlreadyMigrated) {
				progress.AddDone(totalMap[id])
				continue
			}
			return err
		}
		err = st.Close(ctx)
		if err != nil {
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
			objectIds, err := m.storage.GetBoundObjectIds(spaceId)
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

	err = removeFilesWithPrefix(filepath.Join(filepath.Dir(m.path), objectStoreFolder), crdtDb)
	if err != nil {
		return nil
	}
	return renamePreserveExtension(m.oldPath, migratedName)
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

func renamePreserveExtension(oldPath, newName string) error {
	newPath := filepath.Join(filepath.Dir(oldPath), newName+filepath.Ext(oldPath))
	err := os.Rename(oldPath, newPath)
	if err != nil {
		return fmt.Errorf("failed to rename: %w", err)
	}
	return nil
}

func removeFilesWithPrefix(rootDir, prefix string) error {
	return filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasPrefix(info.Name(), prefix) {
			if removeErr := os.Remove(path); removeErr != nil {
				return removeErr
			}
		}
		return nil
	})
}
