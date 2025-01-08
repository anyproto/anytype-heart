package migrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/any-sync/commonspace/spacestorage/migration"

	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/spacecore/oldstorage"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
)

const CName = "client.storage.migration"

const migratedName = "space_store_migrated"

type migrator struct {
	storage    oldstorage.ClientStorage
	newStorage storage.ClientStorage
	process    process.Service
	path       string
	oldPath    string
}

type pathProvider interface {
	GetNewSpaceStorePath() string
	GetOldSpaceStorePath() string
}

func New() app.ComponentRunnable {
	return &migrator{}
}

func (m *migrator) Init(a *app.App) (err error) {
	cfg := a.MustComponent("config").(pathProvider)
	m.path = cfg.GetNewSpaceStorePath()
	m.oldPath = cfg.GetOldSpaceStorePath()
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
	}
	return renamePreserveExtension(m.oldPath, migratedName)
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
