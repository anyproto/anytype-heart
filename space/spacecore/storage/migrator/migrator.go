package migrator

import (
	"context"
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

type migrator struct {
	storage    oldstorage.ClientStorage
	newStorage storage.ClientStorage
	process    process.Service
	path       string
}

type pathProvider interface {
	GetNewSpaceStorePath() string
}

func New() app.ComponentRunnable {
	return &migrator{}
}

func (m *migrator) Init(a *app.App) (err error) {
	m.path = a.MustComponent("config").(pathProvider).GetNewSpaceStorePath()
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
	defer func() {
		progress.Finish(err)
	}()
	if MigrationCompleted(m.path) {
		return nil
	}
	migrator := migration.NewSpaceMigrator(m.storage, m.newStorage, 10)
	allIds, err := m.storage.AllSpaceIds()
	if err != nil {
		return err
	}
	var total int64
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
		err = store.Close(ctx)
		if err != nil {
			return err
		}
	}
	progress.SetTotal(total)
	for _, id := range allIds {
		st, err := migrator.MigrateId(ctx, id, progress)
		if err != nil {
			return err
		}
		err = st.Close(ctx)
		if err != nil {
			return err
		}
	}
	file, err := os.Create(MigrationCompletedPath(m.path))
	if err != nil {
		return err
	}
	return file.Close()
}

func (m *migrator) Close(ctx context.Context) (err error) {
	return nil
}

func MigrationCompletedPath(path string) string {
	return filepath.Join(path, "migration_completed")
}

func MigrationCompleted(path string) bool {
	_, err := os.Stat(MigrationCompletedPath(path))
	return err == nil
}
